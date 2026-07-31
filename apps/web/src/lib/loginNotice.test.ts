import { describe, it, expect } from 'vitest';
import { LOGIN_NOTICES, loginNoticeFor, loginUrlFor } from './loginNotice';

describe('loginNoticeFor', () => {
	it('resolves every known reason to a message', () => {
		for (const reason of Object.keys(LOGIN_NOTICES)) {
			expect(loginNoticeFor(reason)).toBe(LOGIN_NOTICES[reason as keyof typeof LOGIN_NOTICES]);
		}
	});

	it('returns null for absent or unknown reasons', () => {
		expect(loginNoticeFor(null)).toBeNull();
		expect(loginNoticeFor(undefined)).toBeNull();
		expect(loginNoticeFor('')).toBeNull();
		expect(loginNoticeFor('not-a-reason')).toBeNull();
	});

	it('does not resolve inherited Object properties', () => {
		// A hand-typed ?reason=toString must not surface Function.prototype.toString.
		expect(loginNoticeFor('toString')).toBeNull();
		expect(loginNoticeFor('constructor')).toBeNull();
		expect(loginNoticeFor('__proto__')).toBeNull();
	});
});

describe('loginUrlFor', () => {
	it('builds a login URL the load function can read back', () => {
		const url = new URL(loginUrlFor('session-expired'), 'http://localhost:3000');
		expect(url.pathname).toBe('/login');
		expect(loginNoticeFor(url.searchParams.get('reason'))).toBe(
			LOGIN_NOTICES['session-expired']
		);
	});
});
