import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig(() => ({
	plugins: [sveltekit()],
	build: {
		sourcemap: false
	},
	server: {
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true,
				ws: true // Enable WebSocket proxying
			}
		}
	}
}));
