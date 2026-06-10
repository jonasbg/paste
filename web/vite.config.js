import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig(() => ({
	plugins: [svelte()],
	resolve: {
		alias: {
			$lib: fileURLToPath(new URL('./src/lib', import.meta.url))
		}
	},
	// Static assets (favicon, fonts, wasm_exec.js, encryption.wasm, social cards) live
	// in static/ and are copied to the build root — same layout adapter-static produced.
	publicDir: 'static',
	build: {
		outDir: 'build',
		emptyOutDir: true,
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
