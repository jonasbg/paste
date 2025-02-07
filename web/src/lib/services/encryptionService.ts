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
			const ws = new WebSocket(`ws://${window.location.host}/api/ws/upload`);
			const chunkSize = 1024 * 1024; // 1MB chunks
			let offset = 0;

			ws.onopen = async () => {
					// Send header first
					ws.send(header);

					// Send content in chunks
					const content = new Uint8Array(encryptedContent);
					while (offset < content.length) {
							const chunk = content.slice(offset, offset + chunkSize);
							ws.send(chunk);

							offset += chunk.length;
							const progress = Math.round((offset / content.length) * 100);
							await onProgress(progress, `Laster opp... (${progress}%)`);

							// Prevent overwhelming connection
							await new Promise(r => setTimeout(r, 50));
					}

					// Signal end of transmission
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
