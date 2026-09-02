import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	resolve: {
		alias: {
			'$lib': fileURLToPath(new URL('./src/lib', import.meta.url))
		}
	},
	test: {
		// `server/` holds the custom Node server's proxy logic, which runs
		// outside the SvelteKit bundle and is plain JS.
		include: ['src/**/*.test.ts', 'server/**/*.test.js'],
		environment: 'node'
	}
});
