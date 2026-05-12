import { getWasmInstance } from '$lib/utils/wasm-loader';

export async function generateHmacToken(fileId: string, key: string): Promise<string> {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) throw new Error('WASM not initialized');
	if (!wasmInstance.generateHmacToken) throw new Error('generateHmacToken is unavailable');

	return wasmInstance.generateHmacToken(fileId, key);
}
