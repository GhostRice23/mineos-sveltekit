import { describe, it, expect, vi } from 'vitest';
import { EventEmitter } from 'events';
import { WebSocket } from 'ws';

import {
	buildUpstreamHeaders,
	buildUpstreamUrl,
	closeSafely,
	createUpgradeHandler,
	parseAuthToken,
	sanitizeCloseCode,
	sanitizeCloseReason,
	isTrustedUpgradeOrigin,
	shouldProxyWebSocket,
	toWebSocketBase
} from './ws-proxy.js';

const silentLogger = { log: () => {}, error: () => {} };

describe('sanitizeCloseCode', () => {
	it('passes through normal closure', () => {
		expect(sanitizeCloseCode(1000)).toBe(1000);
	});

	it.each([1004, 1005, 1006])('maps unsendable reserved code %i to 1000', (code) => {
		expect(sanitizeCloseCode(code)).toBe(1000);
	});

	it('passes through the other protocol codes', () => {
		for (const code of [1001, 1002, 1003, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014]) {
			expect(sanitizeCloseCode(code)).toBe(code);
		}
	});

	it('passes through the application range', () => {
		expect(sanitizeCloseCode(3000)).toBe(3000);
		expect(sanitizeCloseCode(4999)).toBe(4999);
	});

	it.each([1015, 999, 5000, 0, -1, 1.5, NaN, undefined, null, 'boom'])(
		'falls back to 1000 for %s',
		(code) => {
			expect(sanitizeCloseCode(code)).toBe(1000);
		}
	);

	it('only ever returns codes ws will accept', () => {
		// Mirrors ws/lib/validation.js#isValidStatusCode.
		const isSendable = (c) =>
			(c >= 1000 && c <= 1014 && c !== 1004 && c !== 1005 && c !== 1006) ||
			(c >= 3000 && c <= 4999);
		for (let code = 0; code <= 5100; code++) {
			expect(isSendable(sanitizeCloseCode(code))).toBe(true);
		}
	});
});

describe('sanitizeCloseReason', () => {
	it('returns empty string for missing reasons', () => {
		expect(sanitizeCloseReason(undefined)).toBe('');
		expect(sanitizeCloseReason(null)).toBe('');
		expect(sanitizeCloseReason(Buffer.alloc(0))).toBe('');
	});

	it('decodes buffers', () => {
		expect(sanitizeCloseReason(Buffer.from('bye'))).toBe('bye');
	});

	it('clamps oversized reasons to the 123-byte control frame budget', () => {
		const clamped = sanitizeCloseReason('x'.repeat(500));
		expect(Buffer.byteLength(clamped)).toBeLessThanOrEqual(123);
	});

	it('never splits a multi-byte character', () => {
		const clamped = sanitizeCloseReason('é'.repeat(200));
		expect(Buffer.byteLength(clamped)).toBeLessThanOrEqual(123);
		expect(clamped).toBe(clamped.normalize());
		expect(Buffer.from(clamped, 'utf8').toString('utf8')).toBe(clamped);
	});
});

describe('closeSafely', () => {
	/** Minimal stand-in that enforces the same validation as `ws`. */
	function fakeSocket(readyState = WebSocket.OPEN) {
		return {
			readyState,
			closed: /** @type {any[] | null} */ (null),
			terminated: false,
			close(code, reason) {
				const valid =
					(code >= 1000 && code <= 1014 && code !== 1004 && code !== 1005 && code !== 1006) ||
					(code >= 3000 && code <= 4999);
				if (!valid) throw new TypeError('First argument must be a valid error code number');
				if (Buffer.byteLength(reason ?? '') > 123) {
					throw new RangeError('The message must not be greater than 123 bytes');
				}
				this.closed = [code, reason];
			},
			terminate() {
				this.terminated = true;
			}
		};
	}

	it('does not throw when the peer reported 1006 (issue #156)', () => {
		const socket = fakeSocket();
		expect(() => closeSafely(socket, 1006, Buffer.alloc(0))).not.toThrow();
		expect(socket.closed).toEqual([1000, '']);
		expect(socket.terminated).toBe(false);
	});

	it('does not throw on an oversized reason', () => {
		const socket = fakeSocket();
		expect(() => closeSafely(socket, 1011, 'y'.repeat(400))).not.toThrow();
		expect(socket.closed?.[0]).toBe(1011);
		expect(Buffer.byteLength(socket.closed?.[1])).toBeLessThanOrEqual(123);
	});

	it('falls back to terminate when close still throws', () => {
		const socket = fakeSocket();
		socket.close = () => {
			throw new Error('nope');
		};
		expect(() => closeSafely(socket, 1000)).not.toThrow();
		expect(socket.terminated).toBe(true);
	});

	it('is a no-op for already closing/closed sockets and for nullish input', () => {
		const closing = fakeSocket(WebSocket.CLOSING);
		closeSafely(closing, 1000);
		expect(closing.closed).toBeNull();

		expect(() => closeSafely(null, 1000)).not.toThrow();
		expect(() => closeSafely(undefined, 1000)).not.toThrow();
	});
});

describe('request helpers', () => {
	it('parses the auth cookie', () => {
		expect(parseAuthToken('foo=1; auth_token=abc.def; bar=2')).toBe('abc.def');
		expect(parseAuthToken('foo=1')).toBeNull();
		expect(parseAuthToken(undefined)).toBeNull();
	});

	it('matches only the proxied paths', () => {
		expect(shouldProxyWebSocket('/api/v1/admin/shell/ws')).toBe(true);
		expect(shouldProxyWebSocket('/api/v1/admin/shell/ws?cols=80')).toBe(true);
		expect(shouldProxyWebSocket('/api/v1/admin/shell/wsx')).toBe(false);
		expect(shouldProxyWebSocket('/servers')).toBe(false);
	});

	it('builds upstream URLs without doubling slashes', () => {
		expect(buildUpstreamUrl('ws://api:5078', '/a/b', '?x=1')).toBe('ws://api:5078/a/b?x=1');
		expect(buildUpstreamUrl('ws://api:5078/', '/a/b', '')).toBe('ws://api:5078/a/b');
	});

	it('forwards only the headers that are present', () => {
		expect(buildUpstreamHeaders({ apiKey: 'k', token: 't', protocol: 'p' })).toEqual({
			'X-Api-Key': 'k',
			Authorization: 'Bearer t',
			'Sec-WebSocket-Protocol': 'p'
		});
		expect(buildUpstreamHeaders({ apiKey: '', token: null, protocol: null })).toEqual({});
	});

	it('converts http(s) bases to ws(s)', () => {
		expect(toWebSocketBase('http://api:5078')).toBe('ws://api:5078');
		expect(toWebSocketBase('https://api:5078')).toBe('wss://api:5078');
	});
});

describe('createUpgradeHandler', () => {
	function harness() {
		const upstream = Object.assign(new EventEmitter(), {
			readyState: WebSocket.OPEN,
			close: vi.fn(),
			terminate: vi.fn(),
			send: vi.fn()
		});
		const clientWs = Object.assign(new EventEmitter(), {
			readyState: WebSocket.OPEN,
			close: vi.fn(),
			terminate: vi.fn(),
			send: vi.fn()
		});
		const wss = {
			handleUpgrade: (_req, _socket, _head, cb) => cb(clientWs)
		};
		const socket = Object.assign(new EventEmitter(), {
			write: vi.fn(),
			destroy: vi.fn()
		});
		const handle = createUpgradeHandler({
			wss,
			wsBase: 'ws://api:5078',
			apiKey: 'key',
			logger: silentLogger,
			createUpstream: () => upstream
		});
		const req = { url: '/api/v1/admin/shell/ws', headers: { host: 'localhost:3000' } };
		return { handle, req, socket, upstream, clientWs };
	}

	it('survives an abnormal (1006) close from the upstream', () => {
		const { handle, req, socket, upstream, clientWs } = harness();
		handle(req, socket, Buffer.alloc(0));
		upstream.emit('open');

		expect(() => upstream.emit('close', 1006, Buffer.alloc(0))).not.toThrow();
		expect(clientWs.close).toHaveBeenCalledWith(1000, '');
	});

	it('survives an abnormal (1006) close from the client', () => {
		const { handle, req, socket, upstream, clientWs } = harness();
		handle(req, socket, Buffer.alloc(0));
		upstream.emit('open');

		expect(() => clientWs.emit('close', 1006, Buffer.alloc(0))).not.toThrow();
		expect(upstream.close).toHaveBeenCalledWith(1000, '');
	});

	it('forwards a genuine application close code', () => {
		const { handle, req, socket, upstream, clientWs } = harness();
		handle(req, socket, Buffer.alloc(0));
		upstream.emit('open');

		clientWs.emit('close', 4001, Buffer.from('done'));
		expect(upstream.close).toHaveBeenCalledWith(4001, 'done');
	});

	it('registers an error listener on the raw socket before anything can fail', () => {
		const { handle, req, socket } = harness();
		handle(req, socket, Buffer.alloc(0));
		// An unhandled 'error' on an EventEmitter throws and kills the process.
		expect(() => socket.emit('error', new Error('ECONNRESET'))).not.toThrow();
	});

	it('destroys the raw socket when upstream fails before the handshake', () => {
		const { handle, req, socket, upstream } = harness();
		handle(req, socket, Buffer.alloc(0));
		upstream.emit('error', new Error('ECONNREFUSED'));
		expect(socket.destroy).toHaveBeenCalled();
	});

	it('closes the client instead of destroying the socket after the handshake', () => {
		const { handle, req, socket, upstream, clientWs } = harness();
		handle(req, socket, Buffer.alloc(0));
		upstream.emit('open');
		upstream.emit('error', new Error('reset'));

		expect(socket.destroy).not.toHaveBeenCalled();
		expect(clientWs.close).toHaveBeenCalledWith(1011, 'upstream error');
	});

	it('closes a half-open upstream when the client leaves during connect', () => {
		const { handle, req, socket, upstream } = harness();
		handle(req, socket, Buffer.alloc(0));
		socket.emit('close');
		expect(upstream.close).toHaveBeenCalledWith(1001, 'client gone');
	});

	it('rejects upgrades on non-proxied paths', () => {
		const { handle, socket } = harness();
		handle({ url: '/nope', headers: { host: 'localhost:3000' } }, socket, Buffer.alloc(0));
		expect(socket.write).toHaveBeenCalledWith('HTTP/1.1 404 Not Found\r\n\r\n');
		expect(socket.destroy).toHaveBeenCalled();
	});
});

describe('isTrustedUpgradeOrigin', () => {
	it('allows a handshake with no Origin at all', () => {
		// Browsers always send one; its absence means the Go CLI or curl, which
		// is not what cross-site WebSocket hijacking exploits.
		expect(isTrustedUpgradeOrigin({ host: 'mineos.local' })).toBe(true);
		expect(isTrustedUpgradeOrigin({ origin: null, host: 'mineos.local' })).toBe(true);
		expect(isTrustedUpgradeOrigin({ origin: '', host: 'mineos.local' })).toBe(true);
	});

	it('allows an Origin whose host matches the Host header', () => {
		expect(
			isTrustedUpgradeOrigin({ origin: 'https://mineos.local', host: 'mineos.local' })
		).toBe(true);
	});

	it('ignores the protocol, so TLS termination upstream still works', () => {
		expect(
			isTrustedUpgradeOrigin({ origin: 'https://mineos.local:8443', host: 'mineos.local:8443' })
		).toBe(true);
	});

	it('rejects a different host', () => {
		expect(isTrustedUpgradeOrigin({ origin: 'https://evil.example', host: 'mineos.local' })).toBe(
			false
		);
	});

	it('rejects a host that merely shares a suffix', () => {
		expect(
			isTrustedUpgradeOrigin({ origin: 'https://evil-mineos.local', host: 'mineos.local' })
		).toBe(false);
		expect(
			isTrustedUpgradeOrigin({ origin: 'https://mineos.local.evil.example', host: 'mineos.local' })
		).toBe(false);
	});

	it('rejects a port mismatch', () => {
		expect(
			isTrustedUpgradeOrigin({ origin: 'https://mineos.local:1234', host: 'mineos.local:3000' })
		).toBe(false);
	});

	it('accepts the first X-Forwarded-Host value', () => {
		expect(
			isTrustedUpgradeOrigin({
				origin: 'https://mineos.example',
				host: 'web:3000',
				forwardedHost: 'mineos.example, inner-proxy'
			})
		).toBe(true);
	});

	it('accepts the configured ORIGIN', () => {
		expect(
			isTrustedUpgradeOrigin({
				origin: 'https://mineos.example',
				host: 'web:3000',
				configuredOrigin: 'https://mineos.example'
			})
		).toBe(true);
	});

	it('rejects an Origin carrying userinfo', () => {
		// "https://mineos.local@evil.example" parses to host evil.example; a
		// browser never sends this shape, so treat it as an attack on the parser.
		expect(
			isTrustedUpgradeOrigin({ origin: 'https://mineos.local@evil.example', host: 'mineos.local' })
		).toBe(false);
	});

	it('rejects an unparseable Origin', () => {
		expect(isTrustedUpgradeOrigin({ origin: 'not a url', host: 'mineos.local' })).toBe(false);
		// "null" is what a sandboxed iframe sends — it is not our host.
		expect(isTrustedUpgradeOrigin({ origin: 'null', host: 'mineos.local' })).toBe(false);
	});
});

describe('createUpgradeHandler origin enforcement', () => {
	function harness(configuredOrigin = null) {
		const createUpstream = vi.fn(() =>
			Object.assign(new EventEmitter(), {
				readyState: WebSocket.OPEN,
				close: vi.fn(),
				terminate: vi.fn(),
				send: vi.fn()
			})
		);
		const socket = Object.assign(new EventEmitter(), { write: vi.fn(), destroy: vi.fn() });
		const handle = createUpgradeHandler({
			wss: { handleUpgrade: vi.fn() },
			wsBase: 'ws://api:5078',
			apiKey: 'key',
			configuredOrigin,
			logger: silentLogger,
			createUpstream
		});
		return { handle, socket, createUpstream };
	}

	const shellUpgrade = (headers) => ({ url: '/api/v1/admin/shell/ws', headers });

	it('refuses a cross-origin handshake without contacting the API', () => {
		const { handle, socket, createUpstream } = harness();

		handle(
			shellUpgrade({
				host: 'mineos.local',
				origin: 'https://evil.example',
				cookie: 'auth_token=super-secret'
			}),
			socket,
			Buffer.alloc(0)
		);

		expect(socket.write).toHaveBeenCalledWith('HTTP/1.1 403 Forbidden\r\n\r\n');
		expect(socket.destroy).toHaveBeenCalled();
		// The important half: the operator's cookie is never forwarded upstream.
		expect(createUpstream).not.toHaveBeenCalled();
	});

	it('lets a same-origin handshake through', () => {
		const { handle, socket, createUpstream } = harness();

		handle(
			shellUpgrade({ host: 'mineos.local', origin: 'https://mineos.local' }),
			socket,
			Buffer.alloc(0)
		);

		expect(socket.write).not.toHaveBeenCalled();
		expect(createUpstream).toHaveBeenCalledTimes(1);
	});

	it('lets a non-browser client with no Origin through', () => {
		const { handle, socket, createUpstream } = harness();

		handle(shellUpgrade({ host: 'mineos.local' }), socket, Buffer.alloc(0));

		expect(socket.write).not.toHaveBeenCalled();
		expect(createUpstream).toHaveBeenCalledTimes(1);
	});

	it('honours the configured ORIGIN when Host is the internal name', () => {
		const { handle, socket, createUpstream } = harness('https://mineos.example');

		handle(
			shellUpgrade({ host: 'web:3000', origin: 'https://mineos.example' }),
			socket,
			Buffer.alloc(0)
		);

		expect(createUpstream).toHaveBeenCalledTimes(1);
	});

	it('still 404s an unproxied path before looking at Origin', () => {
		const { handle, socket, createUpstream } = harness();

		handle({ url: '/nope', headers: { host: 'a', origin: 'https://evil.example' } }, socket, Buffer.alloc(0));

		expect(socket.write).toHaveBeenCalledWith('HTTP/1.1 404 Not Found\r\n\r\n');
		expect(createUpstream).not.toHaveBeenCalled();
	});
});
