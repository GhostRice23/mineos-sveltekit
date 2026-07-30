import { describe, it, expect } from 'vitest';
import { isSecureConnection, secureCookieFlag } from './requestProtocol';

describe('isSecureConnection', () => {
	it('trusts X-Forwarded-Proto over the request URL', () => {
		// The docker-compose default: ORIGIN=http://localhost:3000 while the
		// browser actually speaks HTTPS to a TLS-terminating proxy.
		expect(isSecureConnection('https', 'http:')).toBe(true);
		// The mirror image: ORIGIN was set to an https URL but this visitor
		// arrived over plain HTTP. Issuing a Secure cookie here is what made
		// login silently loop (issue #114).
		expect(isSecureConnection('http', 'https:')).toBe(false);
	});

	it('uses the first hop of a comma-joined header', () => {
		expect(isSecureConnection('https, http', 'http:')).toBe(true);
		expect(isSecureConnection('http, https', 'https:')).toBe(false);
	});

	it('is case- and whitespace-insensitive', () => {
		expect(isSecureConnection('  HTTPS  ', 'http:')).toBe(true);
	});

	it('falls back to the URL protocol when the header is absent', () => {
		expect(isSecureConnection(null, 'https:')).toBe(true);
		expect(isSecureConnection(undefined, 'http:')).toBe(false);
		expect(isSecureConnection('', 'https:')).toBe(true);
	});

	it('treats any non-https forwarded scheme as insecure', () => {
		expect(isSecureConnection('ws', 'https:')).toBe(false);
	});
});

describe('secureCookieFlag', () => {
	const req = (headers: Record<string, string> = {}) =>
		new Request('http://localhost:3000/login', { headers });

	it('reads the header off the request', () => {
		expect(secureCookieFlag(req({ 'x-forwarded-proto': 'https' }), new URL('http://a/'))).toBe(
			true
		);
		expect(secureCookieFlag(req(), new URL('http://a/'))).toBe(false);
		expect(secureCookieFlag(req(), new URL('https://a/'))).toBe(true);
	});
});
