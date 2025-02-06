import { getWasmInstance } from '$lib/utils/wasm-loader';

export interface ProgressCallback {
	(progress: number, message: string): Promise<void>;
}

export const MAX_FILE_SIZE = 1 * 1024 * 1024 * 1024; // 1GB

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

	async encryptFile(file: File, key: string, onProgress: ProgressCallback) {
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

			// Yield to event loop before each chunk encryption
			await new Promise((resolve) => setTimeout(resolve, 0));

			const encryptedChunk = wasmInstance.encryptChunk(new Uint8Array(chunk), isLastChunk);
			chunks.push(encryptedChunk);

			// Yield to event loop before progress update
			await new Promise((resolve) => setTimeout(resolve, 0));
			await onProgress(
				10 + (i / totalChunks) * 30,
				`Krypterer... (${Math.round(((i + 1) / totalChunks) * 100)}%)`
			);
		}

		const totalSize = chunks.reduce((acc, chunk) => acc + chunk.length, 0);
		const encryptedContent = new Uint8Array(iv.length + totalSize);
		encryptedContent.set(iv, 0);

		let offset = iv.length;
		for (const chunk of chunks) {
			encryptedContent.set(chunk, offset);
			offset += chunk.length;
		}

		// Yield to event loop before final progress
		await new Promise((resolve) => setTimeout(resolve, 0));
		await onProgress(40, 'Kryptering fullført');

		return { header, encryptedContent };
	}
}
