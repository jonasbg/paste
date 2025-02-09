import { getWasmInstance } from '$lib/utils/wasm-loader';
import { ProgressCallback } from './fileProcessor';

export async function downloadAndDecryptFile(
	fileId: string,
	key: string,
	token: string,
	onProgress: ProgressCallback
): Promise<{ decrypted: Uint8Array; metadata: any }> {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) throw new Error('WASM not initialized');

	await onProgress(0, 'Starting download...');

	// First, fetch just the header to get metadata
	const headerResponse = await fetch(`/api/metadata/${fileId}`, {
		headers: {
			'X-HMAC-Token': token
		}
	});
	if (!headerResponse.ok) {
		throw new Error('Failed to fetch file metadata');
	}

	const headerData = new Uint8Array(await headerResponse.arrayBuffer());
	const metadata = await wasmInstance.decryptMetadata(key, headerData);

	// Now start streaming the full file
	const response = await fetch(`/api/download/${fileId}`, {
		headers: {
			'X-HMAC-Token': token
		}
	});

	if (!response.ok) {
		if (response.status === 403) {
			throw new Error('Invalid access token');
		}
		throw new Error('Failed to download file');
	}

	const reader = response.body!.getReader();
	const contentLength = +(response.headers.get('Content-Length') || 0);
	const decryptedChunks: Uint8Array[] = [];

	let receivedLength = 0;
	let headerProcessed = false;
	let decryptionInitialized = false;
	let bufferedData = new Uint8Array(0);

	// Process the stream
	while (true) {
		const { done, value } = await reader.read();
		if (done) break;

		receivedLength += value.length;

		// // Update download progress
		// if (contentLength > 0) {
		//     const downloadProgress = (receivedLength / contentLength) * 40;
		//     await onProgress(
		//         downloadProgress,
		//         `Downloading... (${Math.round((receivedLength / contentLength) * 100)}%)`
		//     );
		// }

		// Combine buffered data with new chunk
		const newBufferedData = new Uint8Array(bufferedData.length + value.length);
		newBufferedData.set(bufferedData);
		newBufferedData.set(value, bufferedData.length);
		bufferedData = newBufferedData;

		if (!headerProcessed) {
			// Need at least 16 bytes to read metadata length
			if (bufferedData.length < 16) continue;

			const metadataLength = new DataView(bufferedData.buffer).getUint32(12, true);
			const headerLength = 16 + metadataLength;

			// Wait until we have the full header
			if (bufferedData.length < headerLength) continue;

			// Process header and remove it from buffer
			headerProcessed = true;
			bufferedData = bufferedData.slice(headerLength);
		}

		if (!decryptionInitialized && bufferedData.length >= 12) {
			// Initialize decryption with IV
			const iv = bufferedData.slice(0, 12);
			const success = wasmInstance.createDecryptionStream(key, iv);
			if (!success) {
				throw new Error('Failed to initialize decryption stream');
			}

			decryptionInitialized = true;
			bufferedData = bufferedData.slice(12);
		}

		if (decryptionInitialized && bufferedData.length > 0) {
			// Process buffered data in chunks
			const chunkSize = 1024 * 1024 + 16; // 1MB + GCM tag
			while (bufferedData.length >= chunkSize) {
				const chunk = bufferedData.slice(0, chunkSize);
				const isLastChunk = false; // We don't know yet

				const decrypted = wasmInstance.decryptChunk(chunk, isLastChunk);
				if (!decrypted) {
					throw new Error('Failed to decrypt chunk');
				}

				decryptedChunks.push(decrypted);

				const progress = Math.round((decryptedChunks.length * chunkSize / contentLength) * 100);
				await onProgress(
					progress,
					`Laster ned... `
				);

				bufferedData = bufferedData.slice(chunkSize);
			}
		}
	}

	// Process any remaining data
	if (bufferedData.length > 0) {
		const decrypted = wasmInstance.decryptChunk(bufferedData, true);
		if (!decrypted) {
			throw new Error('Failed to decrypt final chunk');
		}
		decryptedChunks.push(decrypted);
	}

	// Create a blob from all decrypted chunks
	const blob = new Blob(decryptedChunks, {
		type: metadata.contentType || 'application/octet-stream'
	});

	await onProgress(100, 'Download and decryption complete');

	return { decrypted: blob, metadata };
}

export async function fetchMetadata(fileId: string, key: string, token: string): Promise<any> {
	try {
		const wasmInstance = getWasmInstance();
		if (!wasmInstance) throw new Error('WASM not initialized');

		const response = await fetch(`/api/metadata/${fileId}`, {
			headers: {
				'X-HMAC-Token': token
			}
		});

		if (response.status === 404) {
			throw new Error('Filen finnes ikke eller har utløpt');
		}

		if (!response.ok) {
			throw new Error('Kunne ikke hente filinformasjon');
		}

		const fileSize = response.headers.get('X-File-Size');
		const encryptedData = await response.arrayBuffer();
		const metadata = wasmInstance.decryptMetadata(key, new Uint8Array(encryptedData));

		if (!metadata.filename) {
			throw new Error("Invalid metadata received");
		}

		return {
			metadata: metadata,
			size: formatFileSize(fileSize ? parseInt(fileSize, 10) : undefined)
		};
	} catch (error) {
		// Improved error handling: Log the error and re-throw (or handle appropriately)
		console.error("Error fetching metadata:", error);
		throw error;  // Re-throw to allow the calling function to handle the error
		// OR, return a default/error value, depending on your needs:
		// return { metadata: null, size: null, error: "Failed to fetch metadata" };
	}
}

function formatFileSize(bytes: number | undefined): string {
	if (!bytes) return '';

	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	let size = bytes;
	let unitIndex = 0;

	while (size >= 1024 && unitIndex < units.length - 1) {
		size /= 1024;
		unitIndex++;
	}

	return `${size.toFixed(1)} ${units[unitIndex]}`;
}