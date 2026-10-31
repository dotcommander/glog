import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import tailwindcss from '@tailwindcss/vite';
import { readFileSync } from 'node:fs';

const { version } = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8'));

/** @type {import('vite').UserConfig} */
const config = {
	plugins: [sveltekit({
		compilerOptions: { runes: true },
		alias: { '$lib': 'src/lib' },
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		version: { name: version },
		paths: { base: '' },
		prerender: {
			handleHttpError: ({ path, message }) => {
				if (path === '/favicon.png') return;
				throw new Error(message);
			}
		}
	}), tailwindcss()],
	server: {
		port: 5173,
		proxy: {
			'/api': {
				target: 'http://localhost:6016',
				changeOrigin: true
			}
		}
	}
};

export default config;
