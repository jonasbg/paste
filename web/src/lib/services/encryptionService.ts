import { generateHmacToken } from '$lib/utils/hmacUtils';
import { getWasmInstance } from '$lib/utils/wasm-loader';
import { FileProcessor, ProgressCallback, MAX_FILE_SIZE } from './fileProcessor';

export function generateKey(): string | null {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) return null;
	return wasmInstance.generateKey();
}

export async function uploadEncryptedFile(
	file: File,
	key: string,
	onProgress: ProgressCallback
): Promise<{ fileId: string; token: string }> {
	if (file.size > MAX_FILE_SIZE) {
		throw new Error(
			`File size exceeds maximum allowed size of ${FileProcessor.formatFileSize(MAX_FILE_SIZE)}`
		);
	}

	const wasmInstance = getWasmInstance();
	if (!wasmInstance) throw new Error('WASM not initialized');

	return new Promise((resolve, reject) => {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/upload`);
		const chunkSize = 1024 * 1024; // 1MB chunks

		let fileOffset = 0;
		let uploadedBytes = 0;
		let currentFileId: string | null = null;

		ws.binaryType = 'arraybuffer';

		ws.onopen = async () => {
			try {
				// Send initial message first - only size needed
				ws.send(
					JSON.stringify({
						type: 'init',
						size: file.size
					})
				);
			} catch (error) {
				reject(new Error('Failed to send initial message: ' + error.message));
				ws.close();
			}
		};

		ws.onmessage = async (event) => {
			if (typeof event.data === 'string') {
				const response = JSON.parse(event.data);
				console.log('WebSocket response:', response);

				if (response.error) {
					reject(new Error(response.error));
					ws.close();
					return;
				}

				// Handle ID response first
				if (response.id && !currentFileId) {
					currentFileId = response.id;
					try {
						// Generate token using server-provided fileId
						const token = await generateHmacToken(currentFileId, key);

						// Send token
						ws.send(
							JSON.stringify({
								type: 'token',
								token: token
							})
						);
					} catch (error) {
						reject(new Error('Failed to generate token: ' + error.message));
						ws.close();
					}
				}

				// After token is accepted, send metadata
				if (response.token_accepted) {
					try {
						// Prepare and send the encrypted metadata header
						const metadata = {
							filename: file.name,
							contentType: file.type,
							size: file.size
						};
						const metadataBytes = new TextEncoder().encode(JSON.stringify(metadata));
						const encryptedMetadata = wasmInstance.encrypt(key, metadataBytes);

						// Format and send header
						const header = new Uint8Array(16 + encryptedMetadata.length - 12);
						header.set(encryptedMetadata.slice(0, 12), 0);
						new DataView(header.buffer).setUint32(12, encryptedMetadata.length - 12, true);
						header.set(encryptedMetadata.slice(12), 16);

						ws.send(header);
					} catch (error) {
						reject(new Error('Failed to prepare header: ' + error.message));
						ws.close();
					}
				}

				// Wait for ready signal before starting encryption stream
				if (response.ready) {
					// Initialize encryption stream and send IV first
					const iv = wasmInstance.createEncryptionStream(key);
					ws.send(iv);
					await sendNextChunk();
				}

				if (response.ack) {
					uploadedBytes += response.ack;
					await onProgress(Math.round((fileOffset / file.size) * 100), `Laster opp...`);
					await sendNextChunk();
				}

				if (response.complete && currentFileId) {
					const token = await generateHmacToken(currentFileId, key);
					resolve({
						fileId: currentFileId,
						token: token
					});
					ws.close();
				}
			}
		};

		ws.onerror = (error) => {
			console.error('WebSocket error:', error);
			reject(new Error('Nettverksfeil under opplasting'));
			ws.close();
		};

		ws.onclose = (event) => {
			console.log('WebSocket closed:', event);
			if (!event.wasClean) {
				reject(new Error('Tilkoblingen ble uventet avbrutt'));
			}
		};

		async function sendNextChunk() {
			try {
				if (fileOffset < file.size) {
					const chunk = await file.slice(fileOffset, fileOffset + chunkSize).arrayBuffer();
					const isLastChunk = fileOffset + chunkSize >= file.size;

					const encryptedChunk = wasmInstance.encryptChunk(new Uint8Array(chunk), isLastChunk);

					ws.send(encryptedChunk);
					fileOffset += chunk.byteLength;
				} else if (fileOffset >= file.size) {
					// Send end-of-transmission marker
					ws.send(new Uint8Array([0]));
				}
			} catch (error) {
				reject(new Error('Feil under kryptering eller sending av data: ' + error.message));
				ws.close();
			}
		}
	});
}
