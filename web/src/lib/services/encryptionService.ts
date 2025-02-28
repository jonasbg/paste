import { generateHmacToken } from '$lib/utils/hmacUtils';
import { getWasmInstance } from '$lib/utils/wasm-loader';
import { FileProcessor, ProgressCallback } from './fileProcessor';
import { configStore } from '$lib/stores/config';
import { get } from 'svelte/store';

export function generateKey(): string | null {
    const wasmInstance = getWasmInstance();
    if (!wasmInstance) return null;

    const config = get(configStore);
    if (!config.data) {
        console.warn('Config not loaded, using default key size of 128');
        return wasmInstance.generateKey(128);
    }

    const keySize = parseInt(config.data.key_size);
    if (isNaN(keySize)) {
        console.warn('Invalid key size in config, using default of 128');
        return wasmInstance.generateKey(128);
    }

    return wasmInstance.generateKey(keySize);
}

export async function uploadEncryptedFile(
    file: File,
    key: string,
    onProgress: ProgressCallback
): Promise<{ fileId: string; token: string }> {
    const fileProcessor = new FileProcessor();
    const config = get(configStore);
    if (config.loading) {
        await new Promise<void>((resolve) => {
            const unsubscribe = configStore.subscribe((state) => {
                if (!state.loading) {
                    unsubscribe();
                    resolve();
                }
            });
        });
    }

    if (config.error) throw new Error(`Failed to load configuration: ${config.error}`);
    if (file.size > fileProcessor.getMaxFileSize()) {
        throw new Error(
            `File size exceeds maximum allowed size of ${FileProcessor.formatFileSize(fileProcessor.getMaxFileSize())}`
        );
    }

    const wasmInstance = getWasmInstance();
    if (!wasmInstance) throw new Error('WASM not initialized');

    return new Promise((resolve, reject) => {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/upload`);
        const chunkSize = 1 * 1024 * 1024; // 4MB chunks
        let fileOffset = 0;
        let uploadedBytes = 0;
        let currentFileId: string | null = null;
        let cachedToken: string | null = null;
        let lastProgress = 0;

        ws.binaryType = 'arraybuffer';

        ws.onopen = async () => {
            ws.send(JSON.stringify({ type: 'init', size: file.size }));
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

                if (response.id && !currentFileId) {
                    currentFileId = response.id;
                    cachedToken = await generateHmacToken(currentFileId, key);
                    ws.send(JSON.stringify({ type: 'token', token: cachedToken }));
                }

                if (response.token_accepted) {
                    const metadata = {
                        filename: file.name,
                        contentType: file.type,
                        size: file.size
                    };
                    const metadataBytes = new TextEncoder().encode(JSON.stringify(metadata));
                    const encryptedMetadata = wasmInstance.encrypt(key, metadataBytes);
                    const header = new Uint8Array(16 + encryptedMetadata.length - 12);
                    header.set(encryptedMetadata.slice(0, 12), 0);
                    new DataView(header.buffer).setUint32(12, encryptedMetadata.length - 12, true);
                    header.set(encryptedMetadata.slice(12), 16);
                    ws.send(header);
                }

                if (response.ready) {
                    const iv = wasmInstance.createEncryptionStream(key);
                    ws.send(iv);
                    await sendNextChunk();
                }

                if (response.ack) {
                    uploadedBytes += response.ack;
                    const progress = Math.round((uploadedBytes / file.size) * 100);
                    if (progress >= lastProgress + 5 || uploadedBytes === file.size) {
                        await onProgress(progress, `Laster opp...`);
                        lastProgress = progress;
                    }
                    await sendNextChunk();
                }

                if (response.complete && currentFileId) {
                    resolve({ fileId: currentFileId, token: cachedToken! });
                    ws.close();
                }
            }
        };

        ws.onerror = () => reject(new Error('Nettverksfeil under opplasting'));
        ws.onclose = (event) => {
            if (!event.wasClean) reject(new Error('Tilkoblingen ble uventet avbrutt'));
        };

        async function sendNextChunk() {
            if (fileOffset < file.size) {
                const chunk = await file.slice(fileOffset, fileOffset + chunkSize).arrayBuffer();
                const isLastChunk = fileOffset + chunkSize >= file.size;
                const encryptedChunk = wasmInstance.encryptChunk(new Uint8Array(chunk), isLastChunk);
                ws.send(encryptedChunk);
                fileOffset += chunk.byteLength;
            }
            else if (fileOffset >= file.size) {
                ws.send(new Uint8Array([0]));
            }
        }
    });
}