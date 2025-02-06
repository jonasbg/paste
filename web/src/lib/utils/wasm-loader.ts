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

let wasmInstance: GoEncryption | null = null;

export async function loadWasmExecutor() {
    if (!browser) return null;

    // Check if Go is already defined
    if (window.Go) {
        return window.Go;
    }

    // Load wasm_exec.js script with proper MIME type
    return new Promise<typeof window.Go>((resolve, reject) => {
        const script = document.createElement('script');
        script.src = '/wasm_exec.js';
        script.type = 'application/javascript';

        // Add additional attributes to help prevent MIME type issues
        script.setAttribute('crossorigin', 'anonymous');
        script.setAttribute('importance', 'high');

        script.onload = () => {
            if (window.Go) {
                resolve(window.Go);
            } else {
                reject(new Error('Go was not defined after script load'));
            }
        };
        script.onerror = (e) => {
            console.error('Script load error:', e);
            reject(new Error('Failed to load wasm_exec.js'));
        };
        document.head.appendChild(script);
    });
}

export async function initWasm(): Promise<GoEncryption> {
    if (!browser) {
        throw new Error('WASM can only be initialized in browser environment');
    }

    if (wasmInstance) return wasmInstance;

    try {
        const Go = await loadWasmExecutor();
        if (!Go) {
            throw new Error('Failed to load Go runtime');
        }

        const go = new Go();

        // Try streaming instantiation first
        try {
            const wasmResponse = await fetch('/encryption.wasm', {
                headers: {
                    'Accept': 'application/wasm',
                    'Content-Type': 'application/wasm'
                }
            });

            if (!wasmResponse.ok) {
                throw new Error(`Failed to fetch WASM: ${wasmResponse.status} ${wasmResponse.statusText}`);
            }

            const result = await WebAssembly.instantiateStreaming(wasmResponse, go.importObject);
            go.run(result.instance);
        } catch (streamingError) {
            console.warn('Streaming instantiation failed, falling back to ArrayBuffer method:', streamingError);

            // Fallback to ArrayBuffer method
            const wasmResponse = await fetch('/encryption.wasm');
            const wasmBuffer = await wasmResponse.arrayBuffer();
            const wasmModule = await WebAssembly.compile(wasmBuffer);
            const instance = await WebAssembly.instantiate(wasmModule, go.importObject);
            go.run(instance);
        }

        // Wait a bit for goEncryption to be initialized
        await new Promise(resolve => setTimeout(resolve, 100));

        if (!window.goEncryption) {
            throw new Error('goEncryption was not initialized after WASM load');
        }

        wasmInstance = window.goEncryption;
        return wasmInstance;
    } catch (error) {
        console.error('Failed to initialize WASM:', error);
        wasmInstance = null;
        throw error;
    }
}

export function getWasmInstance(): GoEncryption | null {
    return wasmInstance;
}
