import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

export default {
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			// Enable generation of .br and .gz variants for static assets
			precompress: true,
			strict: true
		})
	},
	preprocess: vitePreprocess()
};
