import { getWasmInstance } from '$lib/utils/wasm-loader';
import { configStore } from '$lib/stores/config';
import { get } from 'svelte/store';

export interface ProgressCallback {
    (progress: number, message: string): Promise<void>;
}

export class FileProcessor {
    private chunkSize: number;

    constructor(chunkSize = 1 * 1024 * 1024) {
        this.chunkSize = chunkSize;
    }

    static formatFileSize(bytes: number): string {
        const units = ['B', 'KB', 'MB', 'GB'];
        let size = bytes;
        let unitIndex = 0;
        while (size >= 1024 && unitIndex < units.length - 1) {
            size /= 1024;
            unitIndex++;
        }
        return `${size.toFixed(2)} ${units[unitIndex]}`;
    }

    private parseFileSize(sizeStr: string): number {
        const units: { [key: string]: number } = {
            'B': 1,
            'KB': 1024,
            'MB': 1024 * 1024,
            'GB': 1024 * 1024 * 1024,
            'TB': 1024 * 1024 * 1024 * 1024,
        };
        const match = sizeStr.match(/^([\d.]+)\s*([KMGT]?B)$/i);
        if (!match) {
            throw new Error(`Invalid file size format: ${sizeStr}`);
        }
        const [, size, unit] = match;
        return parseFloat(size) * (units[unit.toUpperCase()] || 1);
    }

    private getMaxFileSize(): number {
        const config = get(configStore);
        if (!config.data) {
            throw new Error('Config has not been loaded. Please wait for config to load before processing files.');
        }
        return this.parseFileSize(config.data.max_file_size);
    }

    async encryptFile(file: File, key: string, onProgress: ProgressCallback) {
        // Wait for config to be loaded
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

        // Check file size against maxFileSize
        if (file.size > this.getMaxFileSize()) {
            throw new Error(`File size exceeds the maximum allowed size of ${FileProcessor.formatFileSize(this.getMaxFileSize())}.`);
        }

        const wasmInstance = getWasmInstance();
        if (!wasmInstance) throw new Error('WASM not initialized');

        // Yield to event loop before initial progress
        await new Promise((resolve) => setTimeout(resolve, 0));
        await onProgress(0, 'Forbereder kryptering...');

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

        const iv = wasmInstance.createEncryptionStream(key);
        const chunks = [];
        const totalChunks = Math.ceil(file.size / this.chunkSize);

        for (let i = 0; i < totalChunks; i++) {
            const start = i * this.chunkSize;
            const end = Math.min(start + this.chunkSize, file.size);
            const chunk = await file.slice(start, end).arrayBuffer();
            const isLastChunk = i === totalChunks - 1;

            await new Promise((resolve) => setTimeout(resolve, 0));
            const encryptedChunk = wasmInstance.encryptChunk(new Uint8Array(chunk), isLastChunk);
            chunks.push(encryptedChunk);

            await new Promise((resolve) => setTimeout(resolve, 0));
            await onProgress(Math.round(((i + 1) / totalChunks) * 100), `Laster opp...`);
        }

        const totalSize = chunks.reduce((acc, chunk) => acc + chunk.length, 0);
        const encryptedContent = new Uint8Array(iv.length + totalSize);
        encryptedContent.set(iv, 0);

        let offset = iv.length;
        for (const chunk of chunks) {
            encryptedContent.set(chunk, offset);
            offset += chunk.length;
        }

        return { header, encryptedContent };
    }
}