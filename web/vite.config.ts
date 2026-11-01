import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [
		sveltekit({
			// SPA : le binaire Go sert index.html pour toute route inconnue.
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	server: {
		proxy: {
			'/api': 'http://127.0.0.1:8080'
		}
	},
	test: {
		include: ['src/**/*.test.ts']
	}
});
