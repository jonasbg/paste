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

	const encryptionProgress = (p: number) => onProgress(Math.round(p * 0.5), 'Krypterer...');
	const { header, encryptedContent } = await processor.encryptFile(file, key, encryptionProgress);

	return new Promise((resolve, reject) => {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/upload`);
		const chunkSize = 1024 * 1024;

		let offset = 0;
		let totalSize = encryptedContent.length;
		let fileId: string | null = null;

		ws.onopen = async () => {
			ws.send(header); // Send header first
		};


		ws.onmessage = async (event) => {
            if (typeof event.data === 'string') {
                const response = JSON.parse(event.data);

                if (response.error) {
                    reject(new Error(response.error));
                    ws.close();
                    return;
                }

                if (response.ready) {
                    // Server is ready for the next chunk.  Send it!
                    sendNextChunk();
                }

                if (response.complete) {
					fileId = response.id;
                    await onProgress(100, 'Fullfører...');
                    resolve(response.id);
                    ws.close();
                }

				if(response.ack) {
					//Server has confirmed the sent package
					offset += response.ack; //Important: update with size server got

					const uploadPercent = (offset / totalSize) * 100;
					await onProgress(
						45 + Math.round(uploadPercent * 0.5),
						`Laster opp... (${Math.round(uploadPercent)}%)`
					);

					// Send the next chunk only after acknowledgment.
					sendNextChunk();
				}
            }
        };


		ws.onerror = () => {
            reject(new Error('Nettverksfeil'));
            ws.close();
        };

		async function sendNextChunk() {
			if (offset < totalSize) {
				const chunk = encryptedContent.slice(offset, offset + chunkSize);
				// No longer increment offset here. It moved to the onMessage ack check
				ws.send(chunk);

			} else if (offset >= totalSize && fileId === null) { // important:  only send the end signal IF we haven't already received 'complete'.
				// Send the end-of-transmission marker.
				ws.send(new Uint8Array([0]));
			}
		}
	});
}