import { getWasmInstance } from '$lib/utils/wasm-loader';

export class ChunkedFileProcessor {
    private chunkSize: number;

    constructor(chunkSize = 1 * 1024 * 1024) {
        this.chunkSize = chunkSize;
    }

    async decryptFile(
        encryptedData: Uint8Array,
        key: string,
        progressCallback: (progress: number, message: string) => Promise<void>
    ) {
        const wasmInstance = getWasmInstance();
        if (!wasmInstance) {
            throw new Error('WASM not initialized');
        }

        if (encryptedData.length < 16) {
            throw new Error('Invalid encrypted data');
        }

        try {
            const metadataLength = new DataView(encryptedData.buffer).getUint32(12, true);
            const headerLength = 16 + metadataLength;

            if (encryptedData.length < headerLength) {
                throw new Error('Invalid header length');
            }

            await progressCallback(0, 'Dekrypterer metadata...');
            const metadata = await wasmInstance.decryptMetadata(key, encryptedData.slice(0, headerLength));

            const contentStartPos = headerLength;
            const iv = encryptedData.slice(contentStartPos, contentStartPos + 12);
            const encryptedContent = encryptedData.slice(contentStartPos + 12);

            const success = wasmInstance.createDecryptionStream(key, iv);
            if (!success) {
                throw new Error('Failed to initialize decryption stream');
            }

            const chunks = [];
            const chunkSizeWithTag = this.chunkSize + 16;
            const totalChunks = Math.ceil(encryptedContent.length / chunkSizeWithTag);

            for (let i = 0; i < totalChunks; i++) {
                const start = i * chunkSizeWithTag;
                const end = Math.min(start + chunkSizeWithTag, encryptedContent.length);
                const chunk = encryptedContent.slice(start, end);
                const isLastChunk = i === totalChunks - 1;

                const decryptedChunk = wasmInstance.decryptChunk(chunk, isLastChunk);
                if (!decryptedChunk) {
                    throw new Error(`Failed to decrypt chunk ${i}`);
                }

                chunks.push(decryptedChunk);

                const currentProgress = (i + 1) / totalChunks;
                const scaledProgress = 40 + currentProgress * 50;

                await progressCallback(
                    scaledProgress,
                    `Dekrypterer... (${Math.round(currentProgress * 100)}%)`
                );
            }

            const totalSize = chunks.reduce((acc, chunk) => acc + chunk.length, 0);
            const decrypted = new Uint8Array(totalSize);
            let offset = 0;

            for (const chunk of chunks) {
                decrypted.set(chunk, offset);
                offset += chunk.length;
            }

            return { decrypted, metadata };
        } catch (error) {
            console.error('Decryption error:', error);
            throw error;
        }
    }
}

export async function downloadAndDecryptFile(
    fileId: string,
    encryptionKey: string,
    progressCallback: (progress: number, message: string) => Promise<void>
) {
    const response = await fetch(`/api/download/${fileId}`);
    if (!response.ok) {
        throw new Error('Nedlasting feilet');
    }

    const contentLength = +response.headers.get('Content-Length') || 0;
    const reader = response.body!.getReader();
    const chunks = [];
    let receivedLength = 0;

    while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        chunks.push(value);
        receivedLength += value.length;

        if (contentLength > 0) {
            const downloadProgress = Math.min((receivedLength / contentLength) * 40, 40);
            await progressCallback(
                downloadProgress,
                `Laster ned... (${Math.round((receivedLength / contentLength) * 100)}%)`
            );
        }
    }

    const encryptedData = new Uint8Array(receivedLength);
    let position = 0;
    for (const chunk of chunks) {
        encryptedData.set(chunk, position);
        position += chunk.length;
    }

    await progressCallback(40, 'Dekrypterer...');

    const processor = new ChunkedFileProcessor();
    return processor.decryptFile(encryptedData, encryptionKey, progressCallback);
}

export async function fetchMetadata(fileId: string, encryptionKey: string) {
    const response = await fetch(`/api/metadata/${fileId}`);

    if (response.status === 404) {
        throw new Error('Filen finnes ikke eller har utløpt');
    }

    if (!response.ok) {
        throw new Error('Kunne ikke hente filinformasjon');
    }

    const encryptedData = await response.arrayBuffer();
    if (!encryptionKey) {
        throw new Error('Mangler dekrypteringsnøkkel');
    }

    const wasmInstance = getWasmInstance();
    if (!wasmInstance) {
        throw new Error('WASM not initialized');
    }

    return wasmInstance.decryptMetadata(encryptionKey, new Uint8Array(encryptedData));
}