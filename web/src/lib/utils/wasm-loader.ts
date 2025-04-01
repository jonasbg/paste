import { browser } from '$app/environment';

// Type definitions
interface GoEncryption {
	// Add your encryption methods here
	// Example:
	encrypt?: (data: string) => string;
	decrypt?: (data: string) => string;
}

declare global {
	interface Window {
		Go: any;
		goEncryption: GoEncryption;
	}
}

// Cache name for storing WASM files
const WASM_CACHE_NAME = 'paste-wasm-cache-v1';
const WASM_PATH = '/encryption.wasm';
const WASM_VERSION_KEY = 'wasm-version';
// Update this when your WASM file changes
const CURRENT_WASM_VERSION = '1.0.0';

let wasmInstance: GoEncryption | null = null;
let wasmInitPromise: Promise<GoEncryption> | null = null;

async function loadWasmExecutor() {
	if (!browser) return null;
	if (window.Go) return window.Go;

	return new Promise<typeof window.Go>((resolve, reject) => {
		const script = document.createElement('script');
		script.src = '/wasm_exec.js';
		script.type = 'application/javascript';
		script.setAttribute('crossorigin', 'anonymous');

		script.onload = () => (window.Go ? resolve(window.Go) : reject(new Error('Go not defined')));
		script.onerror = () => reject(new Error('Failed to load wasm_exec.js'));
		document.head.appendChild(script);
	});
}

// Check cache for WASM file
async function getCachedWasm(): Promise<ArrayBuffer | null> {
	if (!('caches' in window)) return null;

	try {
		const cache = await caches.open(WASM_CACHE_NAME);
		const cachedVersion = localStorage.getItem(WASM_VERSION_KEY);

		// Skip cache if version doesn't match
		if (cachedVersion !== CURRENT_WASM_VERSION) {
			return null;
		}

		const cachedResponse = await cache.match(WASM_PATH);
		if (!cachedResponse) return null;

		return await cachedResponse.arrayBuffer();
	} catch (error) {
		console.warn('Failed to retrieve cached WASM:', error);
		return null;
	}
}

// Cache WASM file for future use
async function cacheWasmFile(response: Response): Promise<void> {
	if (!('caches' in window)) return;

	try {
		const cache = await caches.open(WASM_CACHE_NAME);
		await cache.put(WASM_PATH, response.clone());
		localStorage.setItem(WASM_VERSION_KEY, CURRENT_WASM_VERSION);
	} catch (error) {
		console.warn('Failed to cache WASM file:', error);
	}
}

export async function initWasm(): Promise<GoEncryption> {
	if (!browser) {
		throw new Error('WASM can only be initialized in browser environment');
	}

	// Return existing instance or in-progress initialization
	if (wasmInstance) return wasmInstance;
	if (wasmInitPromise) return wasmInitPromise;

	wasmInitPromise = (async () => {
		try {
			const Go = await loadWasmExecutor();
			if (!Go) throw new Error('Failed to load Go runtime');

			const go = new Go();

			// Try to get cached WASM first
			const cachedWasm = await getCachedWasm();

			if (cachedWasm) {
				// Use cached WASM file
				const wasmModule = await WebAssembly.compile(cachedWasm);
				const instance = await WebAssembly.instantiate(wasmModule, go.importObject);
				go.run(instance);
			} else {
				// Download fresh WASM file
				try {
					// Attempt streaming instantiation
					const response = await fetch(WASM_PATH, {
						headers: { Accept: 'application/wasm' }
					});

					if (!response.ok) {
						throw new Error(`Failed to fetch WASM: ${response.status}`);
					}

					// Cache the response
					await cacheWasmFile(response);

					const result = await WebAssembly.instantiateStreaming(response.clone(), go.importObject);
					go.run(result.instance);
				} catch (streamingError) {
					console.warn('Streaming instantiation failed, using fallback method');

					// Fallback to ArrayBuffer method
					const response = await fetch(WASM_PATH);
					const wasmBuffer = await response.arrayBuffer();

					// Cache the response
					await cacheWasmFile(response);

					const wasmModule = await WebAssembly.compile(wasmBuffer);
					const instance = await WebAssembly.instantiate(wasmModule, go.importObject);
					go.run(instance);
				}
			}

			// Use a more reliable method to detect when goEncryption is available
			const waitForGoEncryption = (retries = 10): Promise<GoEncryption> => {
				if (window.goEncryption) return Promise.resolve(window.goEncryption);
				if (retries <= 0) return Promise.reject(new Error('goEncryption not initialized'));

				return new Promise((resolve) => {
					setTimeout(() => {
						resolve(waitForGoEncryption(retries - 1));
					}, 20);
				});
			};

			wasmInstance = await waitForGoEncryption();
			return wasmInstance;
		} catch (error) {
			console.error('Failed to initialize WASM:', error);
			wasmInstance = null;
			wasmInitPromise = null;
			throw error;
		}
	})();

	return wasmInitPromise;
}

export function getWasmInstance(): GoEncryption | null {
	return wasmInstance;
}
