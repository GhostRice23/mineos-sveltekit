// Determines whether the *browser's* connection to MineOS is HTTPS, which is
// what decides if auth cookies may carry the `Secure` attribute.
//
// `event.url.protocol` cannot answer that on its own. adapter-node builds the
// request URL from `origin || get_origin(headers)`:
//   - docker-compose.yml always sets ORIGIN (default `http://localhost:3000`),
//     so url.protocol reports whatever ORIGIN says — not how the browser
//     actually connected;
//   - with ORIGIN unset, get_origin() falls back to `https` unless
//     PROTOCOL_HEADER is configured, which MineOS does not set.
//
// Both directions cause real failures. A `Secure` cookie set for a plain-HTTP
// visitor is silently dropped by the browser: the login POST succeeds, the
// redirect to /servers finds no cookie, and the user is bounced straight back
// to /login — the "page just refreshes, no error" symptom of issue #114. The
// opposite direction is a security bug: behind a TLS-terminating proxy with the
// default ORIGIN, the auth cookie is issued without `Secure` and will be
// replayed over plain HTTP.
//
// `X-Forwarded-Proto` is the only header that reports the browser-facing scheme,
// so it wins when present. It is set by the proxy and is not attacker-controlled
// for direct connections (nothing sets it), and when a proxy is in front, the
// proxy overwrites whatever the client sent.

/**
 * @param forwardedProto raw `X-Forwarded-Proto` header, if any
 * @param urlProtocol `event.url.protocol` (e.g. `'https:'`), used as fallback
 */
export function isSecureConnection(
	forwardedProto: string | null | undefined,
	urlProtocol: string
): boolean {
	const proto = forwardedProto?.split(',')[0].trim().toLowerCase();
	if (proto) return proto === 'https';
	return urlProtocol === 'https:';
}

/**
 * The `Secure` flag to use for auth cookies on this request.
 */
export function secureCookieFlag(request: Request, url: URL): boolean {
	return isSecureConnection(request.headers.get('x-forwarded-proto'), url.protocol);
}
