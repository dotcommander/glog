import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { readFileSync } from 'node:fs';

const { version } = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8'));

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	extensions: ['.svelte'],
	compilerOptions: {
		runes: true
	},
	kit: {
		version: {
			name: version
		},
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html', // SPA mode - all routes fallback to index.html
			precompress: false,
			strict: true
		}),
		paths: {
			base: '' // Serve from root
		},
		prerender: {
			handleHttpError: ({ path, message }) => {
				// Ignore missing favicon
				if (path === '/favicon.png') return;
				throw new Error(message);
			}
		}
	}
};

export default config;
