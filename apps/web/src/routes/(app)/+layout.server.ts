import { redirect, isRedirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';
import * as api from '$lib/api/client';
import { loginUrlFor } from '$lib/loginNotice';

export const load: LayoutServerLoad = async ({ cookies, fetch, url }) => {
	const token = cookies.get('auth_token');

	if (!token) {
		throw redirect(303, '/login');
	}

	// Every bounce below carries a reason and logs a line, so a login loop is
	// diagnosable instead of looking like the page simply refreshed (#114).
	let user = null;
	try {
		const meResponse = await fetch('/api/auth/me');
		if (meResponse.ok) {
			user = await meResponse.json();
		} else if (meResponse.status === 401 || meResponse.status === 403) {
			console.warn(`[auth] /api/auth/me rejected the session (${meResponse.status})`);
			throw redirect(303, loginUrlFor('session-expired'));
		} else {
			console.warn(`[auth] /api/auth/me returned ${meResponse.status}`);
			throw redirect(303, loginUrlFor('api-unreachable'));
		}
	} catch (err) {
		if (isRedirect(err)) throw err;
		console.error('[auth] /api/auth/me request failed:', err);
		throw redirect(303, loginUrlFor('api-unreachable'));
	}

	if (!user) {
		// Could not get user info, force re-login
		console.warn('[auth] /api/auth/me returned no user');
		throw redirect(303, loginUrlFor('session-expired'));
	}

	if (url.pathname.startsWith('/profiles/buildtools')) {
		console.info('[layout] buildtools load', url.pathname, url.search);
	}

	// Load servers and profiles for search
	const [servers, profiles] = await Promise.all([
		api.getAllServers(fetch),
		api.getHostProfiles(fetch)
	]);

	return {
		user,
		servers: servers.data ?? [],
		profiles: profiles.data ?? []
	};
};
