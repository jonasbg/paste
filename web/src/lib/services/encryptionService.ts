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

	const encryptionProgress = (p: number) => onProgress(Math.round(p * 0.4), 'Krypterer...');
	const { header, encryptedContent } = await processor.encryptFile(file, key, encryptionProgress);

	return new Promise((resolve, reject) => {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/upload`);
		const chunkSize = 1024 * 1024;

		let offset = 0;
		const content = new Uint8Array(encryptedContent);

		ws.onopen = async () => {
			ws.send(header);
			sendNextChunk();
		};

		const sendNextChunk = () => {
			if (offset < content.length) {
				const chunk = content.slice(offset, offset + chunkSize);
				ws.send(chunk);
				offset += chunk.length;
			} else {
				ws.send(new Uint8Array([0]));
			}
		};

		ws.onmessage = async (event) => {
			const response = JSON.parse(event.data);

			if (response.error) {
				reject(new Error(response.error));
			} else if (response.progress) {
				const serverProgress = response.progress * 0.6;
				await onProgress(45 + Math.round(serverProgress), 'Laster opp...');
				sendNextChunk();
			} else if (response.complete) {
				await onProgress(100, 'Fullført');
				resolve(response.id);
			}
		};

		ws.onerror = () => reject(new Error('Nettverksfeil'));
	});
}