// Why the app bounced someone back to /login.
//
// Before this existed every guard redirected to a bare `/login`, so a failed
// session looked identical to a fresh visit: the page simply re-rendered with
// no message and nothing in the logs (issue #114). Carrying a reason turns a
// silent refresh loop into something a user can act on.

export const LOGIN_NOTICES = {
	'session-expired': 'Your session has expired. Please sign in again.',
	'api-unreachable':
		'The MineOS API could not be reached, so your session could not be verified. Check that the API container is running, then try again.',
	'signed-out': 'You have been signed out.'
} as const;

export type LoginNoticeReason = keyof typeof LOGIN_NOTICES;

/**
 * Resolve a `?reason=` query value to a user-facing message.
 * Unknown values yield `null` so a hand-typed URL cannot inject text.
 */
export function loginNoticeFor(reason: string | null | undefined): string | null {
	if (!reason) return null;
	return Object.prototype.hasOwnProperty.call(LOGIN_NOTICES, reason)
		? LOGIN_NOTICES[reason as LoginNoticeReason]
		: null;
}

/** Build the login URL carrying a bounce reason. */
export function loginUrlFor(reason: LoginNoticeReason): string {
	return `/login?reason=${reason}`;
}
