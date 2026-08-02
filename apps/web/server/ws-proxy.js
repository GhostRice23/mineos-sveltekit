// WebSocket proxy plumbing for the custom Node server (../server.js).
//
// Kept in its own side-effect-free module so the close-code/URL/header logic is
// unit-testable without binding a port. See ws-proxy.test.js.

import { WebSocket } from 'ws';

/** Paths that are proxied to the API as WebSocket connections. */
export const WS_PROXY_PATHS = ['/api/v1/admin/shell/ws'];

/** Maximum byte length of a WebSocket close reason (RFC 6455 control frame limit). */
const MAX_CLOSE_REASON_BYTES = 123;

/**
 * Parse the auth token out of a Cookie header.
 * @param {string | undefined | null} cookieHeader
 * @returns {string | null}
 */
export function parseAuthToken(cookieHeader) {
	if (!cookieHeader) return null;
	const match = cookieHeader.match(/auth_token=([^;]+)/);
	return match?.[1] || null;
}

/**
 * Host of a URL-ish string, or null when it is not usable.
 * @param {string | null | undefined} value
 * @returns {string | null}
 */
function hostOf(value) {
	if (!value) return null;
	try {
		const parsed = new URL(value);
		// Browsers never send userinfo in Origin; a userinfo-bearing value is
		// either garbage or an attempt to confuse the parser. Reject it.
		if (parsed.username || parsed.password) return null;
		return parsed.host.toLowerCase();
	} catch {
		return null;
	}
}

/**
 * Same-origin check for the WebSocket handshake.
 *
 * hooks.server.ts guards SvelteKit requests, but an upgrade never reaches
 * SvelteKit — it is answered off the raw 'upgrade' event — so without this the
 * only proxied path (`/api/v1/admin/shell/ws`, the admin shell) had no origin
 * check at all. Cookies ride along on a cross-site handshake the same way they
 * do on any subresource request, so any page a signed-in operator visited could
 * open a shell as them: cross-site WebSocket hijacking.
 *
 * Host matching mirrors isTrustedOrigin in src/lib/server/originCheck.ts, and
 * deliberately ignores protocol for the same reason: behind a TLS-terminating
 * proxy the browser sends https:// while the internal request is http://.
 * The logic is duplicated rather than imported because this module runs as
 * plain Node ESM against the built output, with no access to src/.
 *
 * A missing Origin is allowed. Browsers always send one on a WebSocket
 * handshake, so its absence means a non-browser client (the Go CLI, curl) —
 * and those are not what CSWSH exploits. Requiring it would break them for no
 * gain in defence.
 *
 * @param {{ origin?: string | null, host?: string | null, forwardedHost?: string | null, configuredOrigin?: string | null }} input
 * @returns {boolean}
 */
export function isTrustedUpgradeOrigin({
	origin,
	host,
	forwardedHost,
	configuredOrigin
} = {}) {
	if (origin === null || origin === undefined || origin === '') return true;

	const originHost = hostOf(origin);
	if (!originHost) return false;

	if (host && originHost === host.trim().toLowerCase()) return true;

	const forwarded = forwardedHost?.split(',')[0].trim().toLowerCase();
	if (forwarded && originHost === forwarded) return true;

	const configuredHost = hostOf(configuredOrigin);
	if (configuredHost && originHost === configuredHost) return true;

	return false;
}

/**
 * Check if a path should be WebSocket proxied.
 * @param {string} path
 * @returns {boolean}
 */
export function shouldProxyWebSocket(path) {
	return WS_PROXY_PATHS.some((p) => path === p || path.startsWith(p + '?'));
}

/**
 * Map a *received* close code onto one that is legal to *send*.
 *
 * A peer's close event reports codes that must never be echoed back onto the
 * wire: 1005 (no status received) and 1006 (abnormal closure) are synthesised
 * locally and are rejected by `ws` with
 * `TypeError: First argument must be a valid error code number`. Before this
 * mapping existed the proxy forwarded them verbatim, and because the throw
 * happened inside a `'close'` listener nothing could catch it — it took the
 * whole mineos-web process down while the API kept running (issue #156).
 *
 * Sendable codes are 1000-1014 except 1004/1005/1006, plus the application
 * range 3000-4999; anything else degrades to a normal closure.
 *
 * @param {unknown} code
 * @returns {number} a code accepted by `WebSocket#close`
 */
export function sanitizeCloseCode(code) {
	if (typeof code !== 'number' || !Number.isInteger(code)) return 1000;
	if (code >= 3000 && code <= 4999) return code;
	if (code >= 1000 && code <= 1014 && code !== 1004 && code !== 1005 && code !== 1006) {
		return code;
	}
	return 1000;
}

/**
 * Clamp a close reason to the 123-byte control-frame budget. Oversized reasons
 * make `ws` throw `RangeError`, which would crash the process the same way an
 * invalid code does.
 *
 * @param {unknown} reason
 * @returns {string} a reason that always fits in a close frame
 */
export function sanitizeCloseReason(reason) {
	if (reason === undefined || reason === null) return '';

	let text;
	if (typeof reason === 'string') {
		text = reason;
	} else if (Buffer.isBuffer(reason)) {
		text = reason.toString('utf8');
	} else {
		return '';
	}

	if (Buffer.byteLength(text) <= MAX_CLOSE_REASON_BYTES) return text;

	// Drop trailing characters until it fits, so a multi-byte character is
	// never cut in half.
	let end = text.length;
	while (end > 0 && Buffer.byteLength(text.slice(0, end)) > MAX_CLOSE_REASON_BYTES) {
		end--;
	}
	return text.slice(0, end);
}

/**
 * Close a socket without ever throwing at the caller. Falls back to
 * `terminate()` so a socket that refuses a graceful close is still torn down.
 *
 * @param {import('ws').WebSocket | null | undefined} socket
 * @param {unknown} [code]
 * @param {unknown} [reason]
 */
export function closeSafely(socket, code, reason) {
	if (!socket) return;
	if (socket.readyState === WebSocket.CLOSING || socket.readyState === WebSocket.CLOSED) {
		return;
	}
	try {
		socket.close(sanitizeCloseCode(code), sanitizeCloseReason(reason));
	} catch {
		try {
			socket.terminate();
		} catch {
			// Nothing left to do — the socket is already gone.
		}
	}
}

/**
 * Build the upstream WebSocket URL for a proxied request.
 * @param {string} wsBase
 * @param {string} path
 * @param {string} search
 * @returns {string}
 */
export function buildUpstreamUrl(wsBase, path, search) {
	return `${wsBase.replace(/\/+$/, '')}${path}${search}`;
}

/**
 * Build the headers forwarded to the upstream API.
 * @param {{ apiKey?: string, token?: string | null, protocol?: string | null }} input
 * @returns {Record<string, string>}
 */
export function buildUpstreamHeaders({ apiKey, token, protocol }) {
	/** @type {Record<string, string>} */
	const headers = {};
	if (apiKey) headers['X-Api-Key'] = apiKey;
	if (token) headers['Authorization'] = `Bearer ${token}`;
	if (protocol) headers['Sec-WebSocket-Protocol'] = protocol;
	return headers;
}

/**
 * Convert an http(s) base URL into its ws(s) equivalent.
 * @param {string} apiBase
 * @returns {string}
 */
export function toWebSocketBase(apiBase) {
	return apiBase.replace(/^http/, 'ws');
}

/**
 * @typedef {object} UpgradeHandlerOptions
 * @property {import('ws').WebSocketServer} wss
 * @property {string} wsBase Upstream base URL, already in ws:// form.
 * @property {string} [apiKey]
 * @property {string | null} [configuredOrigin] Value of the ORIGIN env var, if set.
 * @property {Pick<Console, 'log' | 'error'>} [logger]
 * @property {(url: string, headers: Record<string, string>) => import('ws').WebSocket} [createUpstream]
 */

/**
 * Build the `server.on('upgrade')` handler.
 *
 * Every socket involved gets an `'error'` listener before it can emit one:
 * an unhandled `'error'` on an `EventEmitter` throws, and during an upgrade
 * that throw escapes to the top level and kills the process.
 *
 * @param {UpgradeHandlerOptions} options
 * @returns {(req: import('http').IncomingMessage, socket: import('stream').Duplex, head: Buffer) => void}
 */
export function createUpgradeHandler({
	wss,
	wsBase,
	apiKey = '',
	configuredOrigin = null,
	logger = console,
	createUpstream = (url, headers) => new WebSocket(url, { headers })
}) {
	return function handleUpgrade(req, socket, head) {
		const url = new URL(req.url || '/', `http://${req.headers.host}`);
		const path = url.pathname;

		// The raw socket can fail at any point (client hangs up mid-handshake,
		// network reset). Without this listener that surfaces as an unhandled
		// 'error' event and takes the process down.
		socket.on('error', (err) => {
			logger.error(`[WS Proxy] Client socket error:`, err.message);
		});

		if (!shouldProxyWebSocket(path)) {
			// SvelteKit doesn't handle WebSocket upgrades.
			logger.log(`[WS] Rejecting WebSocket upgrade for: ${path}`);
			socket.write('HTTP/1.1 404 Not Found\r\n\r\n');
			socket.destroy();
			return;
		}

		// Checked before the cookie is read, so a cross-site handshake never gets
		// as far as borrowing the operator's session.
		if (
			!isTrustedUpgradeOrigin({
				origin: req.headers.origin,
				host: req.headers.host,
				forwardedHost: /** @type {string | undefined} */ (req.headers['x-forwarded-host']),
				configuredOrigin
			})
		) {
			logger.error(
				`[WS Proxy] Blocked cross-origin upgrade for ${path}: ` +
					`Origin="${req.headers.origin ?? ''}" Host="${req.headers.host ?? ''}"`
			);
			socket.write('HTTP/1.1 403 Forbidden\r\n\r\n');
			socket.destroy();
			return;
		}

		const token = parseAuthToken(req.headers.cookie);
		const targetUrl = buildUpstreamUrl(wsBase, path, url.search);

		logger.log(`[WS Proxy] Connecting to: ${targetUrl}`);

		const upstream = createUpstream(
			targetUrl,
			buildUpstreamHeaders({
				apiKey,
				token,
				protocol: /** @type {string | undefined} */ (req.headers['sec-websocket-protocol']) ?? null
			})
		);

		// Until the handshake completes there is no client WebSocket to close,
		// only the raw socket. Tracking it keeps the pre-handshake failure path
		// (destroy the raw socket) from firing after the upgrade, where it would
		// abort an otherwise healthy connection.
		/** @type {import('ws').WebSocket | null} */
		let client = null;

		upstream.on('open', () => {
			logger.log(`[WS Proxy] Connected to upstream`);

			wss.handleUpgrade(req, socket, head, (clientWs) => {
				client = clientWs;

				clientWs.on('message', (data, isBinary) => {
					if (upstream.readyState === WebSocket.OPEN) {
						upstream.send(data, { binary: isBinary });
					}
				});

				upstream.on('message', (data, isBinary) => {
					if (clientWs.readyState === WebSocket.OPEN) {
						clientWs.send(data, { binary: isBinary });
					}
				});

				clientWs.on('close', (code, reason) => {
					logger.log(`[WS Proxy] Client closed: ${code}`);
					closeSafely(upstream, code, reason);
				});

				upstream.on('close', (code, reason) => {
					logger.log(`[WS Proxy] Upstream closed: ${code}`);
					closeSafely(clientWs, code, reason);
				});

				clientWs.on('error', (err) => {
					logger.error(`[WS Proxy] Client error:`, err.message);
					closeSafely(upstream, 1011, 'client error');
				});
			});
		});

		upstream.on('error', (err) => {
			logger.error(`[WS Proxy] Upstream error:`, err.message);
			if (client) {
				// The client is a real WebSocket now; tear it down gracefully
				// instead of yanking the underlying socket out from under it.
				closeSafely(client, 1011, 'upstream error');
				return;
			}
			socket.destroy();
		});

		// Client gave up before the upstream finished connecting — don't leak
		// the half-open upstream connection.
		socket.on('close', () => {
			if (!client) {
				closeSafely(upstream, 1001, 'client gone');
			}
		});
	};
}
