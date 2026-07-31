import { redirect, fail, isRedirect } from '@sveltejs/kit';
import { loginNoticeFor } from '$lib/loginNotice';
import { secureCookieFlag } from '$lib/server/requestProtocol';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ cookies, url }) => {
	const token = cookies.get('auth_token');
	if (token) {
		throw redirect(303, '/servers');
	}
	return { notice: loginNoticeFor(url.searchParams.get('reason')) };
};

export const actions = {
	default: async ({ request, cookies, fetch, url }) => {
		const data = await request.formData();
		const username = data.get('username')?.toString();
		const password = data.get('password')?.toString();

		if (!username || !password) {
			return fail(400, { error: 'Username and password are required' });
		}

		try {
			const response = await fetch('/api/auth/login', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ username, password })
			});

			if (!response.ok) {
				if (response.status === 401) {
					return fail(401, { error: 'Invalid username or password' });
				}
				return fail(response.status, { error: 'Login failed. Please try again.' });
			}

			const result = await response.json();

			// Derived from the browser-facing scheme, not ORIGIN — a `Secure`
			// cookie issued to a plain-HTTP visitor is dropped silently and the
			// user is bounced back to /login with no error (issue #114).
			const secure = secureCookieFlag(request, url);

			// Set httpOnly cookie with the JWT token
			// User info is loaded server-side via /api/auth/me in layout.server.ts
			cookies.set('auth_token', result.accessToken, {
				httpOnly: true,
				secure,
				sameSite: 'lax',
				maxAge: result.expiresInSeconds,
				path: '/'
			});

			throw redirect(303, '/servers');
		} catch (err) {
			// SvelteKit signals the redirect above by throwing; re-throw it.
			if (isRedirect(err)) {
				throw err;
			}
			console.error('Login error:', err);
			return fail(500, { error: 'An unexpected error occurred' });
		}
	}
} satisfies Actions;
