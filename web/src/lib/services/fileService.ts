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

	await onProgress(0, 'Laster ned...');

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

				const progress = Math.round(((decryptedChunks.length * chunkSize) / contentLength) * 100);
				await onProgress(progress, `Laster ned... `);

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

	await onProgress(100, 'Nedlasting og dekryptering fullført');

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
			throw new Error('Invalid metadata received');
		}

		return {
			metadata: metadata,
			size: formatFileSize(fileSize ? parseInt(fileSize, 10) : undefined)
		};
	} catch (error) {
		// Improved error handling: Log the error and re-throw (or handle appropriately)
		console.error('Error fetching metadata:', error);
		throw error; // Re-throw to allow the calling function to handle the error
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

export async function streamDownloadAndDecrypt(
  fileId: string,
  key: string,
  token: string,
  onProgress: ProgressCallback
): Promise<{ stream: ReadableStream<Uint8Array>; metadata: any }> {
  const wasmInstance = getWasmInstance();
  if (!wasmInstance) throw new Error('WASM not initialized');

  await onProgress(0, 'Laster ned...');

  // First, fetch metadata as before...
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
      'X-HMAC-Token': token,
      'Connection': 'keep-alive' // Explicitly request persistent connection
    }
  });

  if (!response.ok) {
    if (response.status === 403) {
      throw new Error('Invalid access token');
    }
    throw new Error('Failed to download file');
  }

  if (!response.body) {
    throw new Error('Response body is null');
  }

  const contentLength = +(response.headers.get('Content-Length') || 0);
  let receivedLength = 0;
  let headerProcessed = false;
  let decryptionInitialized = false;
  let bufferedData = new Uint8Array(0);

  let lastProgressUpdate = 0;
  let lastProgressValue = 0;
  const PROGRESS_THROTTLE_MS = 100;

  const reportProgress = (bytesReceived: number) => {
    const currentTime = Date.now();
    const progressValue = contentLength > 0
      ? Math.round((bytesReceived / contentLength) * 100)
      : 0;

    if (
      currentTime - lastProgressUpdate > PROGRESS_THROTTLE_MS ||
      Math.abs(progressValue - lastProgressValue) >= 1 ||
      progressValue === 100
    ) {
      lastProgressUpdate = currentTime;
      lastProgressValue = progressValue;

      // Use queueMicrotask instead of setTimeout for more reliable scheduling
      queueMicrotask(() => {
        onProgress(progressValue, `Laster ned...`).catch(console.error);
      });
    }
  };

  // Create a transform stream with optimized buffer handling
  const decryptionStream = new TransformStream<Uint8Array, Uint8Array>({
    start() {
      // Initialize stream state
      headerProcessed = false;
      decryptionInitialized = false;
      bufferedData = new Uint8Array(0);
    },

    transform: (chunk, controller) => {
      try {
        // Add incoming chunk to our buffer
        receivedLength += chunk.length;
        reportProgress(receivedLength);

        // More efficient buffer concatenation
        const newBuffer = new Uint8Array(bufferedData.length + chunk.length);
        newBuffer.set(bufferedData);
        newBuffer.set(chunk, bufferedData.length);
        bufferedData = newBuffer;

        // Process header data
        if (!headerProcessed && bufferedData.length >= 16) {
          const metadataLength = new DataView(bufferedData.buffer, bufferedData.byteOffset + 12, 4).getUint32(0, true);
          const headerLength = 16 + metadataLength;

          if (bufferedData.length >= headerLength) {
            headerProcessed = true;
            bufferedData = bufferedData.slice(headerLength);
          }
        }

        // Initialize decryption when IV is available
        if (headerProcessed && !decryptionInitialized && bufferedData.length >= 12) {
          const iv = bufferedData.slice(0, 12);
          if (!wasmInstance.createDecryptionStream(key, iv)) {
            throw new Error('Failed to initialize decryption stream');
          }

          decryptionInitialized = true;
          bufferedData = bufferedData.slice(12);
        }

        // Process complete chunks when possible
        if (decryptionInitialized) {
          const chunkSize = 1024 * 1024 + 16; // 1MB + GCM tag

          while (bufferedData.length >= chunkSize) {
            const dataChunk = bufferedData.slice(0, chunkSize);
            const decrypted = wasmInstance.decryptChunk(dataChunk, false);

            if (!decrypted) {
              throw new Error('Failed to decrypt chunk');
            }

            controller.enqueue(decrypted);
            bufferedData = bufferedData.slice(chunkSize);
          }
        }
      } catch (error) {
        console.error("Transform error:", error);
        controller.error(error);
      }
    },

    flush: (controller) => {
      try {
        // Process final data and signal completion
        if (decryptionInitialized && bufferedData.length > 0) {
          const decrypted = wasmInstance.decryptChunk(bufferedData, true);
          if (decrypted) {
            controller.enqueue(decrypted);
          }
        }

        // Final progress update
        queueMicrotask(() => {
          onProgress(100, 'Nedlasting fullført').catch(console.error);
        });
      } catch (error) {
        console.error("Flush error:", error);
        controller.error(error);
      }
    }
  });

  // Create a resilient stream that handles network interruptions
  const decryptedStream = response.body
    .pipeThrough(new TransformStream({
      transform(chunk, controller) {
        // Pass through data but catch any network errors
        try {
          controller.enqueue(chunk);
        } catch (error) {
          console.error("Network stream error:", error);
          controller.error(error);
        }
      }
    }))
    .pipeThrough(decryptionStream);

  return { stream: decryptedStream, metadata };
}
