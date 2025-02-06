<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { initWasm, getWasmInstance } from '$lib/utils/wasm-loader';

	let isWasmLoaded = false;
	let isLoading = false;
	let encryptionKey: string = '';
	let metadata: any = null;
	let downloadProgress = 0;
	let downloadMessage = '';
	let isDownloading = false;
	let downloadError: string | null = null;
	let isDownloadComplete = false;

	// DOM element bindings
	let downloadContainer: HTMLElement;
	let downloadProgressBar: HTMLElement;
	let downloadProgressText: HTMLElement;

	class ChunkedFileProcessor {
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
				// Handle metadata first
				const metadataLength = new DataView(encryptedData.buffer).getUint32(12, true);
				const headerLength = 16 + metadataLength;

				if (encryptedData.length < headerLength) {
					throw new Error('Invalid header length');
				}

				await progressCallback(0, 'Dekrypterer metadata...');
				const metadata = await wasmInstance.decryptMetadata(
					key,
					encryptedData.slice(0, headerLength)
				);

				// Get the IV and encrypted content
				const contentStartPos = headerLength;
				const iv = encryptedData.slice(contentStartPos, contentStartPos + 12);
				const encryptedContent = encryptedData.slice(contentStartPos + 12);

				// Initialize decryption stream with IV
				const success = wasmInstance.createDecryptionStream(key, iv);
				if (!success) {
					throw new Error('Failed to initialize decryption stream');
				}

				// Process content in chunks
				const chunks = [];
				const chunkSizeWithTag = this.chunkSize + 16; // AES-GCM tag size is 16 bytes
				const totalChunks = Math.ceil(encryptedContent.length / chunkSizeWithTag);

				// Process chunks with controlled timing
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

				// Combine all decrypted chunks
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

	async function updateProgress(progress: number, message: string) {
		downloadProgress = progress;
		downloadMessage = message;

		if (downloadProgressBar) {
			downloadProgressBar.style.width = `${progress}%`;
		}
		if (downloadProgressText) {
			downloadProgressText.textContent = `${Math.round(progress)}%`;
		}
	}

	async function fetchFileMetadata() {
		try {
			const fileId = $page.params.fileId;
			const response = await fetch(`/api/metadata/${fileId}`);

			if (response.status === 404) {
				downloadError = 'Filen finnes ikke eller har utløpt';
				return;
			}

			if (!response.ok) {
				throw new Error('Kunne ikke hente filinformasjon');
			}

			const encryptedData = await response.arrayBuffer();
			if (!encryptionKey) {
				metadata = { error: 'Mangler dekrypteringsnøkkel' };
				return;
			}

			const wasmInstance = getWasmInstance();
			if (!wasmInstance) {
				throw new Error('WASM not initialized');
			}

			metadata = await wasmInstance.decryptMetadata(encryptionKey, new Uint8Array(encryptedData));
		} catch (error) {
			console.error('Metadata error:', error);
			metadata = { error: 'Kunne ikke dekryptere filinformasjon' };
		}
	}

	async function downloadFile() {
		if (!encryptionKey) {
			downloadError = 'Mangler dekrypteringsnøkkel';
			return;
		}

		if (isDownloading) return;
		isDownloading = true;
		downloadError = null;

		try {
			const fileId = $page.params.fileId;
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
					await updateProgress(
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

			await updateProgress(40, 'Dekrypterer...');

			const processor = new ChunkedFileProcessor();
			const { decrypted, metadata } = await processor.decryptFile(
				encryptedData,
				encryptionKey,
				updateProgress
			);

			await updateProgress(90, 'Forbereder nedlasting...');

			const blob = new Blob([decrypted], {
				type: metadata.contentType || 'application/octet-stream'
			});
			const url = window.URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = metadata.filename;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			window.URL.revokeObjectURL(url);

			isDownloadComplete = true;
			await updateProgress(100, 'Fullført!');
		} catch (error) {
			console.error('Download error:', error);
			downloadError = (error as Error).message;
		} finally {
			isDownloading = false;
		}
	}

	onMount(async () => {
		if (!browser) return;

		try {
			await initWasm();
			isWasmLoaded = true;

			const urlParams = new URLSearchParams(window.location.hash.slice(1));
			encryptionKey = urlParams.get('key') || '';

			await fetchFileMetadata();
		} catch (error) {
			console.error('Failed to initialize:', error);
			downloadError = 'Failed to initialize the application';
		}
	});
</script>

<div class="container">
	<div class="download-container" bind:this={downloadContainer}>
		<h1>Last ned fil</h1>

		{#if downloadError}
			<div class="error-message">
				<svg
					xmlns="http://www.w3.org/2000/svg"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<circle cx="12" cy="12" r="10" />
					<line x1="12" y1="8" x2="12" y2="12" />
					<line x1="12" y1="16" x2="12.01" y2="16" />
				</svg>
				<p>{downloadError}</p>
			</div>
		{:else if !metadata}
			<div class="file-info">
				<div class="file-info-item">Laster filinformasjon...</div>
			</div>
		{:else if metadata.error}
			<div class="error-message">
				<svg
					xmlns="http://www.w3.org/2000/svg"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<circle cx="12" cy="12" r="10" />
					<line x1="12" y1="8" x2="12" y2="12" />
					<line x1="12" y1="16" x2="12.01" y2="16" />
				</svg>
				<p>{metadata.error}</p>
			</div>
		{:else}
			<div class="file-info">
				<div class="file-info-item">Fil: {metadata.filename}</div>
			</div>

			{#if !isDownloadComplete}
				<button class="button" on:click={downloadFile} disabled={isDownloading}>
					{isDownloading ? 'Laster ned...' : 'Last ned'}
				</button>
			{/if}

			{#if isDownloading || isDownloadComplete}
				<div class="download-progress">
					<div class="progress-title">{downloadMessage}</div>
					<div class="download-progress-bar">
						<div
							class="download-progress-fill"
							bind:this={downloadProgressBar}
							style="width: {downloadProgress}%"
						></div>
					</div>
					<div class="download-progress-text" bind:this={downloadProgressText}>
						{downloadProgress}%
					</div>
				</div>
			{/if}

			{#if isDownloadComplete}
				<div class="success-message">
					<svg
						xmlns="http://www.w3.org/2000/svg"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
						<polyline points="22 4 12 14.01 9 11.01" />
					</svg>
					<p>
						Filen er lastet ned og sikkert slettet fra serveren vår. Takk for at du bruker vår sikre
						fildelingstjeneste!
					</p>
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.container {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
	}

	h1 {
		font-size: 2.5rem;
		font-weight: 500;
		margin-bottom: 1.5rem;
	}

	.download-container {
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 2rem;
	}

	.file-info {
		margin: 1rem 0;
		padding: 1rem;
		background-color: white;
		border-radius: var(--border-radius);
	}

	.success-message {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 16px;
		background-color: #f0fdf4;
		border: 1px solid #dcfce7;
		border-radius: 8px;
		margin-top: 16px;
	}

	.success-message svg {
		flex-shrink: 0;
		width: 24px;
		height: 24px;
		color: #16a34a;
	}

	.success-message p {
		margin: 0;
		color: #166534;
		font-weight: 500;
		line-height: 1.5;
	}

	.file-info-item {
		margin: 0.5rem 0;
		color: #666;
	}

	.download-progress {
		margin-top: 1rem;
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 1rem;
	}

	.download-progress-bar {
		width: 100%;
		height: 8px;
		background-color: #e0e0e0;
		border-radius: 4px;
		overflow: hidden;
		margin-top: 0.5rem;
	}

	.download-progress-fill {
		width: 0%;
		height: 100%;
		background-color: var(--primary-green);
		transition: width 0.3s ease-in-out;
	}

	.download-progress-text {
		margin-top: 0.5rem;
		font-size: 0.875rem;
		color: #666;
	}

	.button {
		background-color: var(--primary-green);
		color: white;
		border: none;
		border-radius: var(--border-radius);
		padding: 0.75rem 1.5rem;
		font-size: 1rem;
		cursor: pointer;
		transition: background-color 0.2s;
		font-weight: 500;
		margin: 1rem 0;
	}

	.button:hover {
		background-color: #359669;
	}

	.button:disabled {
		background-color: #e0e0e0;
		cursor: not-allowed;
	}

	.success-message,
	.error-message {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 16px;
		border-radius: 8px;
		margin-top: 16px;
	}

	.success-message {
		background-color: #f0fdf4;
		border: 1px solid #dcfce7;
	}

	.error-message {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 16px;
		background-color: #fef2f2;
		border: 1px solid #fee2e2;
		border-radius: 8px;
		margin-top: 16px;
	}

	.error-message svg {
		flex-shrink: 0;
		width: 24px;
		height: 24px;
		color: #dc2626;
	}

	.error-message p {
		margin: 0;
		color: #991b1b;
		font-weight: 500;
		line-height: 1.5;
	}

	.key-prompt {
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 2rem;
		margin-top: 1rem;
	}

	.key-prompt h2 {
		font-size: 1.25rem;
		margin: 0 0 1rem 0;
		font-weight: 500;
	}

	.key-prompt p {
		margin-bottom: 1rem;
		color: #666;
	}

	.key-input {
		width: 100%;
		padding: 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		margin-bottom: 1rem;
		font-family: inherit;
	}

	.key-input:focus {
		outline: none;
		border-color: var(--primary-green);
		box-shadow: 0 0 0 2px rgba(64, 184, 123, 0.2);
	}
</style>
