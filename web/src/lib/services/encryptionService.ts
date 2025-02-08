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
): Promise<string> {
    if (file.size > MAX_FILE_SIZE) {
        throw new Error(`File size exceeds maximum allowed size of ${FileProcessor.formatFileSize(MAX_FILE_SIZE)}`);
    }

    const wasmInstance = getWasmInstance();
    if (!wasmInstance) throw new Error('WASM not initialized');

    return new Promise((resolve, reject) => {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/upload`);
        const chunkSize = 1024 * 1024; // 1MB chunks

        let fileOffset = 0;
        let uploadedBytes = 0;
        let fileId: string | null = null;

        ws.binaryType = 'arraybuffer';

        ws.onopen = async () => {
            try {
                // Create metadata header first
                const metadata = {
                    filename: file.name,
                    contentType: file.type,
                    size: file.size
                };
                const metadataBytes = new TextEncoder().encode(JSON.stringify(metadata));
                const encryptedMetadata = wasmInstance.encrypt(key, metadataBytes);

                // Format header according to expected format
                // [16 bytes header][encrypted metadata][IV][encrypted content]
                const header = new Uint8Array(16 + encryptedMetadata.length - 12);
                header.set(encryptedMetadata.slice(0, 12), 0); // IV from metadata encryption
                new DataView(header.buffer).setUint32(12, encryptedMetadata.length - 12, true);
                header.set(encryptedMetadata.slice(12), 16); // Encrypted metadata without IV

                ws.send(header);
            } catch (error) {
                reject(new Error('Failed to prepare header: ' + error.message));
                ws.close();
            }
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
                    // Initialize encryption stream and send IV first
                    const iv = wasmInstance.createEncryptionStream(key);
                    ws.send(iv);
                    await sendNextChunk();
                }

                if (response.complete) {
                    fileId = response.id;
                    await onProgress(100, 'Fullført');
                    resolve(response.id);
                    ws.close();
                }

                if (response.ack) {
                    uploadedBytes += response.ack;
                    await onProgress(
											Math.round((fileOffset / file.size) * 100),
                        `Laster opp...`
                    );

                    await sendNextChunk();
                }
            }
        };

        ws.onerror = () => {
            reject(new Error('Nettverksfeil under opplasting'));
            ws.close();
        };

        ws.onclose = (event) => {
            if (!event.wasClean && !fileId) {
                reject(new Error('Tilkoblingen ble uventet avbrutt'));
            }
        };

        async function sendNextChunk() {
            try {
                if (fileOffset < file.size) {
                    const chunk = await file.slice(fileOffset, fileOffset + chunkSize).arrayBuffer();
                    const isLastChunk = fileOffset + chunkSize >= file.size;

                    // Encrypt the chunk
                    const encryptedChunk = wasmInstance.encryptChunk(
                        new Uint8Array(chunk),
                        isLastChunk
                    );

                    // Send the encrypted chunk
                    ws.send(encryptedChunk);
                    fileOffset += chunk.byteLength;
                } else if (fileOffset >= file.size && fileId === null) {
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