import type { Cookies } from '@sveltejs/kit';

type CookieOptions = {
	path: string;
	domain?: string;
	secure?: boolean;
	sameSite?: 'lax' | 'strict' | 'none';
};

const baseOptions = (overrides: Partial<CookieOptions>): CookieOptions => ({
	path: '/',
	sameSite: 'lax',
	...overrides
});

const hostCandidates = (hostname: string): Array<Pick<CookieOptions, 'domain'>> => {
	const candidates: Array<Pick<CookieOptions, 'domain'>> = [{}, { domain: hostname }];
	if (
		hostname.includes('.') &&
		hostname !== 'localhost' &&
		!hostname.match(/^\d+\.\d+\.\d+\.\d+$/)
	) {
		candidates.push({ domain: `.${hostname}` });
	}
	return candidates;
};

export const clearAuthCookies = (cookies: Cookies, url: URL) => {
	// Always clear both variants. url.protocol reflects ORIGIN rather than the
	// browser's scheme (see requestProtocol.ts), so gating on it used to leave a
	// `Secure` cookie behind and the user apparently still signed in. Emitting
	// an expiry the browser ignores is harmless; missing one is not.
	const secureOptions = [true, false];
	const hosts = hostCandidates(url.hostname);

	for (const host of hosts) {
		for (const secure of secureOptions) {
			const opts = baseOptions({ ...host, secure });
			cookies.delete('auth_token', opts);
			cookies.delete('auth_user', opts);
		}
	}
};
