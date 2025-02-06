import { browser } from '$app/environment';

let wasmInstance: any = null;

export async function loadWasmExecutor() {
	if (!browser) return null;

	// Check if Go is already defined
	if ((window as any).Go) {
		return (window as any).Go;
	}

	// Load wasm_exec.js script with proper MIME type
	return new Promise((resolve, reject) => {
		const script = document.createElement('script');
		script.src = '/wasm_exec.js';
		script.type = 'application/javascript'; // Explicitly set MIME type
		script.onload = () => {
			resolve((window as any).Go);
		};
		script.onerror = (e) => {
			console.error('Script load error:', e);
			reject(new Error('Failed to load wasm_exec.js'));
		};
		document.head.appendChild(script);
	});
}

export async function initWasm() {
	if (!browser || wasmInstance) return wasmInstance;

	try {
		const Go = await loadWasmExecutor();
		if (!Go) {
			throw new Error('Failed to load Go runtime');
		}

		const go = new Go();

		// Fetch WASM with proper content type
		const wasmResponse = await fetch('/encryption.wasm', {
			headers: {
				Accept: 'application/wasm'
			}
		});

		const result = await WebAssembly.instantiateStreaming(wasmResponse, go.importObject);

		go.run(result.instance);
		wasmInstance = (window as any).goEncryption;
		return wasmInstance;
	} catch (error) {
		console.error('Failed to initialize WASM:', error);
		throw error;
	}
}

export function getWasmInstance() {
	return wasmInstance;
}
