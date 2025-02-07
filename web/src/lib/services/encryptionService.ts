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
	await new Promise((resolve) => setTimeout(resolve, 0));

	const { header, encryptedContent } = await processor.encryptFile(file, key, onProgress);
	await onProgress(50, 'Laster opp...');

	return new Promise((resolve, reject) => {
			const ws = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/ws/upload`);

			ws.onopen = async () => {
					try {
							// Send the entire encrypted file in one message
							ws.send(new Blob([header, encryptedContent]));
							const chunkSize = 32 * 1024; // 32KB chunks
							let offset = 0;
							const content = new Uint8Array(encryptedContent);

							while (offset < content.length) {
									const chunk = content.slice(offset, offset + chunkSize);
									ws.send(chunk);

									const percentComplete = Math.round((offset / content.length) * 50) + 50;
									await onProgress(
											percentComplete,
											`Laster opp... (${Math.round((offset / content.length) * 100)}%)`
									);

									offset += chunkSize;
									// Prevent overwhelming the connection
									await new Promise(resolve => setTimeout(resolve, 0));
							}
					} catch (error) {
							ws.close();
							reject(new Error('Feil under opplasting'));
					}
			};

			ws.onmessage = async (event) => {
					try {
							const response = JSON.parse(event.data);
							if (response.error) {
									reject(new Error(response.error));
							} else if (response.complete) {
									await onProgress(100, 'Fullført!');
									resolve(response.id);
							}
					} catch (error) {
							reject(new Error('Kunne ikke tolke server-respons'));
					}
			};

			ws.onerror = () => {
					reject(new Error('Nettverksfeil under opplasting'));
			};

			ws.onclose = (event) => {
					if (!event.wasClean) {
							reject(new Error('Tilkoblingen ble uventet avbrutt'));
					}
			};
	});
}
