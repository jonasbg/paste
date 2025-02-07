import { getWasmInstance } from '$lib/utils/wasm-loader';
import { FileProcessor } from './fileProcessor';

export function generateKey(): string | null {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) return null;

	const key = wasmInstance.generateKey();
	return key;
}

export async function uploadEncryptedFile(
	file: File,
	key: string,
	onProgress: (progress: number, message: string) => Promise<void>
): Promise<string> {
	const processor = new FileProcessor();
	const { header, encryptedContent } = await processor.encryptFile(file, key, onProgress);

	return new Promise((resolve, reject) => {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/upload`);
		const chunkSize = 1024 * 1024; // 1MB chunks

		let uploadProgress = 0;
		let offset = 0;

		ws.onopen = async () => {
			ws.send(header);

			// Send content in chunks
			const content = new Uint8Array(encryptedContent);
			while (offset < content.length) {
				const chunk = content.slice(offset, offset + chunkSize);
				ws.send(chunk);

				offset += chunk.length;
				uploadProgress = 50 + Math.round((offset / content.length) * 50);
				await onProgress(uploadProgress, `Laster opp... (${Math.round((offset / content.length) * 100)}%)`);

				await new Promise(r => setTimeout(r, 10));
			}

			await onProgress(100, 'Fullfører...');
			ws.send(new Uint8Array([0]));
		};

		ws.onmessage = async (event) => {
			const response = JSON.parse(event.data);
			if (response.error) reject(new Error(response.error));
			if (response.complete) resolve(response.id);
		};

		ws.onerror = () => reject(new Error('Nettverksfeil'));
	});
}
