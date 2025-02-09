import { getWasmInstance } from '$lib/utils/wasm-loader';

export async function generateHmacToken(fileId: string, key: string): Promise<string> {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) throw new Error('WASM not initialized');

	return wasmInstance.generateHmacToken(fileId, key);
}
