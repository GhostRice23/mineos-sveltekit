import { defineConfig } from 'vitest/config';

export default defineConfig({
	test: {
		// `server/` holds the custom Node server's proxy logic, which runs
		// outside the SvelteKit bundle and is plain JS.
		include: ['src/**/*.test.ts', 'server/**/*.test.js'],
		environment: 'node'
	}
});
