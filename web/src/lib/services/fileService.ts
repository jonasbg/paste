import { getWasmInstance } from '$lib/utils/wasm-loader';
export async function downloadAndDecryptFile(
	fileId: string,
	key: string,
	onProgress: ProgressCallback
): Promise<{ decrypted: Uint8Array; metadata: any }> {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) throw new Error('WASM not initialized');

	try {
		// Yield to event loop to allow initial progress update
		await new Promise((resolve) => setTimeout(resolve, 0));
		await onProgress(0, 'Starting download...');

		const response = await fetch(`/api/download/${fileId}`);
		if (!response.ok) throw new Error('Download failed');

		const contentLength = +response.headers.get('Content-Length') || 0;
		const reader = response.body!.getReader();
		const chunks = [];
		let receivedLength = 0;

		while (true) {
			const { done, value } = await reader.read();
			if (done) break;

			chunks.push(value);
			receivedLength += value.length;

			// Yield to event loop and update progress
			await new Promise((resolve) => setTimeout(resolve, 0));
			if (contentLength > 0) {
				const downloadProgress = (receivedLength / contentLength) * 40;
				await onProgress(
					downloadProgress,
					`Downloading... (${Math.round((receivedLength / contentLength) * 100)}%)`
				);
			}
		}

		// Combine chunks
		const encryptedData = new Uint8Array(receivedLength);
		let position = 0;
		for (const chunk of chunks) {
			encryptedData.set(chunk, position);
			position += chunk.length;
		}

		// Decrypt metadata
		await new Promise((resolve) => setTimeout(resolve, 0));

		const metadataLength = new DataView(encryptedData.buffer).getUint32(12, true);
		const headerLength = 16 + metadataLength;

		if (encryptedData.length < headerLength) {
			throw new Error('Invalid header length');
		}

		const metadata = await wasmInstance.decryptMetadata(key, encryptedData.slice(0, headerLength));

		// Prepare for content decryption
		const contentStartPos = headerLength;
		const iv = encryptedData.slice(contentStartPos, contentStartPos + 12);
		const encryptedContent = encryptedData.slice(contentStartPos + 12);

		const success = wasmInstance.createDecryptionStream(key, iv);
		if (!success) {
			throw new Error('Failed to initialize decryption stream');
		}

		// Process content in chunks with more granular progress updates
		const chunkSize = 1 * 1024 * 1024; // 1MB chunks
		const chunkSizeWithTag = chunkSize + 16; // AES-GCM tag size is 16 bytes
		const totalChunks = Math.ceil(encryptedContent.length / chunkSizeWithTag);
		const decryptedChunks = [];

		for (let i = 0; i < totalChunks; i++) {
			const start = i * chunkSizeWithTag;
			const end = Math.min(start + chunkSizeWithTag, encryptedContent.length);
			const chunk = encryptedContent.slice(start, end);
			const isLastChunk = i === totalChunks - 1;

			// Yield to event loop before each chunk decryption
			await new Promise((resolve) => setTimeout(resolve, 0));

			const decryptedChunk = wasmInstance.decryptChunk(chunk, isLastChunk);
			if (!decryptedChunk) {
				throw new Error(`Failed to decrypt chunk ${i}`);
			}

			decryptedChunks.push(decryptedChunk);

			const currentProgress = (i + 1) / totalChunks;
			const scaledProgress = 40 + currentProgress * 60;

			// Ensure progress update with event loop yield
			await new Promise((resolve) => setTimeout(resolve, 0));
			await onProgress(
				Math.round(scaledProgress),
				`Decrypting... (${Math.round(currentProgress * 100)}%)`
			);
		}

		// Combine decrypted chunks
		const totalSize = decryptedChunks.reduce((acc, chunk) => acc + chunk.length, 0);
		const decrypted = new Uint8Array(totalSize);
		let offset = 0;

		for (const chunk of decryptedChunks) {
			decrypted.set(chunk, offset);
			offset += chunk.length;
		}

		// Final progress update
		await new Promise((resolve) => setTimeout(resolve, 0));
		await onProgress(100, 'Decryption complete');
		return { decrypted, metadata };
	} catch (error) {
		console.error('Decryption error:', error);
		throw error;
	}
}

export async function fetchMetadata(fileId: string, key: string): Promise<any> {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) throw new Error('WASM not initialized');

	const response = await fetch(`/api/metadata/${fileId}`);

	if (response.status === 404) {
		throw new Error('Filen finnes ikke eller har utløpt');
	}

	if (!response.ok) {
		throw new Error('Kunne ikke hente filinformasjon');
	}

	const encryptedData = await response.arrayBuffer();
	return wasmInstance.decryptMetadata(key, new Uint8Array(encryptedData));
}
