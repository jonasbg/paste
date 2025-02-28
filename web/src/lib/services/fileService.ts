import { getWasmInstance } from '$lib/utils/wasm-loader';
import { ProgressCallback } from './fileProcessor';

export async function streamDownloadAndDecryptWS(
  fileId: string,
  key: string,
  token: string,
  onProgress: ProgressCallback
): Promise<{ stream: ReadableStream<Uint8Array>; metadata: any }> {
  const wasmInstance = getWasmInstance();
  if (!wasmInstance) throw new Error('WASM not initialized');

  await onProgress(0, 'Initialiserer...');

  // Still fetch metadata using the HTTP endpoint
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

  // Create a stream for transferring data from WebSocket to decryption process
  const { readable, writable } = new TransformStream<Uint8Array, Uint8Array>();
  const writer = writable.getWriter();

  // Start the WebSocket connection
  const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/ws/download`;
  console.log(`Connecting to WebSocket at: ${wsUrl}`);
  const ws = new WebSocket(wsUrl);

  ws.binaryType = 'arraybuffer';

  let fileSize = 0;
  let receivedSize = 0;
  let lastProgressUpdate = 0;

  // Set up WebSocket event handlers
  ws.onopen = () => {
    try {
      console.log("WebSocket connection opened, sending download_init");
      // Send initial request
      ws.send(JSON.stringify({
        type: 'download_init',
        fileId: fileId,
        token: token
      }));
    } catch (error) {
      console.error("Error in onopen:", error);
      writer.abort(error as Error);
      ws.close();
    }
  };

  ws.onmessage = async (event) => {
    try {
      if (typeof event.data === 'string') {
        // Handle JSON control messages
        console.log("Received string message:", event.data);
        const message = JSON.parse(event.data);

        if (message.type === 'file_info') {
          fileSize = message.size;
          console.log(`File size: ${fileSize} bytes`);
          await onProgress(0, 'Starter nedlasting...');

          // Tell server we're ready to receive data
          ws.send(JSON.stringify({
            type: 'ready',
            ready: true
          }));
        }
        else if (message.type === 'complete') {
          console.log("Download complete, sending acknowledgment");
          // Send final acknowledgment
          ws.send(JSON.stringify({
            type: 'complete_ack',
            complete: true
          }));

          // Close the writer to signal end of stream
          writer.close();
          await onProgress(100, 'Nedlasting fullført');
        }
        else if (message.error) {
          console.error("Error message from server:", message.error);
          throw new Error(message.error);
        }
      }
      else {
        // Binary data - this is a chunk of the file
        const chunk = new Uint8Array(event.data);
        receivedSize += chunk.length;

        // Update progress
        const currentTime = Date.now();
        const progressValue = fileSize > 0 ? Math.round((receivedSize / fileSize) * 100) : 0;

        if (currentTime - lastProgressUpdate > 100 || progressValue === 100) {
          lastProgressUpdate = currentTime;
          await onProgress(progressValue, `Laster ned...`);
        }

        // Write chunk to the stream
        await writer.write(chunk);

        // Send acknowledgment back to server
        ws.send(JSON.stringify({
          type: 'ack',
          size: chunk.length
        }));
      }
    } catch (error) {
      console.error("Error in onmessage:", error);
      writer.abort(error as Error);
      ws.close();
    }
  };

  ws.onerror = (event) => {
    console.error("WebSocket error occurred");
    writer.abort(new Error('WebSocket error occurred'));
    try {
      ws.close();
    } catch (e) {
      // Ignore if already closed
    }
  };

  ws.onclose = (event) => {
    console.log(`WebSocket closed with code ${event.code}`);
    if (!event.wasClean) {
      writer.abort(new Error(`Connection closed unexpectedly (code: ${event.code})`));
    } else if (writer.desiredSize !== null) {
      // If the writer hasn't been closed yet, close it
      writer.close();
    }
  };

  // Create decryption transform stream
  const decryptionStream = new TransformStream({
    start(controller) {
      this.headerProcessed = false;
      this.decryptionInitialized = false;
      this.bufferedData = new Uint8Array(0);
      console.log("Decryption transform started");
    },

    transform(chunk, controller) {
      try {
        // Add incoming chunk to our buffer
        const newBuffer = new Uint8Array(this.bufferedData.length + chunk.length);
        newBuffer.set(this.bufferedData);
        newBuffer.set(chunk, this.bufferedData.length);
        this.bufferedData = newBuffer;

        console.log(`Received chunk of ${chunk.length} bytes, buffer now ${this.bufferedData.length} bytes`);

        // Process header data if needed
        if (!this.headerProcessed && this.bufferedData.length >= 16) {
          console.log("Processing header");
          const metadataLength = new DataView(this.bufferedData.buffer, this.bufferedData.byteOffset + 12, 4).getUint32(0, true);
          console.log(`Metadata length: ${metadataLength}`);
          const headerLength = 16 + metadataLength;

          if (this.bufferedData.length >= headerLength) {
            console.log(`Header processed (${headerLength} bytes)`);
            this.headerProcessed = true;
            this.bufferedData = this.bufferedData.slice(headerLength);
          }
        }

        // Initialize decryption with IV
        if (this.headerProcessed && !this.decryptionInitialized && this.bufferedData.length >= 12) {
          console.log("Initializing decryption with IV");
          const iv = this.bufferedData.slice(0, 12);
          if (!wasmInstance.createDecryptionStream(key, iv)) {
            throw new Error('Failed to initialize decryption stream');
          }

          console.log("Decryption initialized");
          this.decryptionInitialized = true;
          this.bufferedData = this.bufferedData.slice(12);
        }

        // Process chunks
        if (this.decryptionInitialized) {
          const chunkSize = 1024 * 1024 + 16; // 1MB + GCM tag

          while (this.bufferedData.length >= chunkSize) {
            console.log(`Processing ${chunkSize} byte chunk`);
            const dataChunk = this.bufferedData.slice(0, chunkSize);
            const decrypted = wasmInstance.decryptChunk(dataChunk, false);

            if (!decrypted) {
              throw new Error('Failed to decrypt chunk');
            }

            console.log(`Decrypted chunk size: ${decrypted.length} bytes`);
            controller.enqueue(decrypted);
            this.bufferedData = this.bufferedData.slice(chunkSize);
          }
        }
      } catch (error) {
        console.error("Transform error:", error);
        controller.error(error);
      }
    },

    flush(controller) {
      try {
        // Process any final data
        if (this.decryptionInitialized && this.bufferedData.length > 0) {
          console.log(`Processing final chunk of ${this.bufferedData.length} bytes`);
          const decrypted = wasmInstance.decryptChunk(this.bufferedData, true);
          if (decrypted) {
            console.log(`Final decrypted chunk: ${decrypted.length} bytes`);
            controller.enqueue(decrypted);
          }
        }
        console.log("Decryption transform completed");
      } catch (error) {
        console.error("Flush error:", error);
        controller.error(error);
      }
    }
  });

  // Use the decryption stream to process data
  return {
    stream: readable.pipeThrough(decryptionStream),
    metadata
  };
}

// Update the main download function to use WebSockets when available
export async function streamDownloadAndDecrypt(
  fileId: string,
  key: string,
  token: string,
  onProgress: ProgressCallback
): Promise<{ stream: ReadableStream<Uint8Array>; metadata: any }> {
  try {
    console.log("Starting download with WebSockets");
    if ('WebSocket' in window) {
      return await streamDownloadAndDecryptWS(fileId, key, token, onProgress);
    } else {
      console.log("WebSockets not supported, using HTTP fallback");
      // Use HTTP version for browsers without WebSocket support
      const { decrypted, metadata } = await legacyDownloadAndDecryptFile(fileId, key, token, onProgress);

      // Convert to a ReadableStream to maintain consistent interface
      const stream = new ReadableStream({
        start(controller) {
          controller.enqueue(decrypted);
          controller.close();
        }
      });

      return { stream, metadata };
    }
  } catch (error) {
    console.error('Download error:', error);
    throw error;
  }
}

async function legacyDownloadAndDecryptFile(
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
