<!-- src/routes/+page.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { initWasm, getWasmInstance } from '$lib/utils/wasm-loader';

	let encryptionKey: string;
  let isLoading = false;
	let isWasmLoaded = false;
	const maxFileSize = 1 * 1024 * 1024 * 1024; // 1GB in bytes

	let fileInput: HTMLInputElement;
	let uploadSection: HTMLElement;
	let downloadContainer: HTMLElement;
	let selectedFile: HTMLElement;
	let urlField: HTMLInputElement;
	let downloadFileName: HTMLElement;
	let progressContainer: HTMLElement;
	let progressTitle: HTMLElement;
	let progressBar: HTMLElement;
	let progressText: HTMLElement;
	let downloadProgress: HTMLElement;
	let downloadProgressTitle: HTMLElement;
	let downloadProgressBar: HTMLElement;
	let downloadProgressText: HTMLElement;
	let finishedDownloading: HTMLElement;
	let fileInfo: HTMLElement;
	let fileNameInfo: HTMLElement;
	let fileSizeInfo: HTMLElement;
	let urlContainer: HTMLElement;

	class ChunkedFileProcessor {
		constructor(chunkSize = 1 * 1024 * 1024) {
			this.chunkSize = chunkSize;
		}

		async encryptFile(file, key) {
			const wasmInstance = getWasmInstance();
			await updateProgress(0, 'Forbereder kryptering...');

			// Encrypt metadata first
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

			// Initialize streaming encryption
			const iv = wasmInstance.createEncryptionStream(key);
			const chunks = [];
			const totalChunks = Math.ceil(file.size / this.chunkSize);

			// Process file in chunks
			for (let i = 0; i < totalChunks; i++) {
				const start = i * this.chunkSize;
				const end = Math.min(start + this.chunkSize, file.size);
				const chunk = await file.slice(start, end).arrayBuffer();
				const isLastChunk = i === totalChunks - 1;

				const encryptedChunk = wasmInstance.encryptChunk(new Uint8Array(chunk), isLastChunk);

				chunks.push(encryptedChunk);

				await updateProgress(
					10 + (i / totalChunks) * 30,
					`Krypterer... (${Math.round(((i + 1) / totalChunks) * 100)}%)`
				);
			}

			// Combine all encrypted chunks
			const totalSize = chunks.reduce((acc, chunk) => acc + chunk.length, 0);
			const encryptedContent = new Uint8Array(iv.length + totalSize);
			encryptedContent.set(iv, 0);

			let offset = iv.length;
			for (const chunk of chunks) {
				encryptedContent.set(chunk, offset);
				offset += chunk.length;
			}

			console.log('Final sizes:', {
				header: header.length,
				iv: iv.length,
				encryptedContent: encryptedContent.length,
				total: header.length + encryptedContent.length
			});

			await updateProgress(40, 'Kryptering fullført');

			return { header, encryptedContent };
		}

		async decryptFile(encryptedData, key, progressCallback) {
			const wasmInstance = getWasmInstance();
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

				console.log('Decryption details:', {
					totalLength: encryptedContent.length,
					chunkSize: this.chunkSize,
					chunkSizeWithTag,
					totalChunks
				});

				// Process chunks with controlled timing
				for (let i = 0; i < totalChunks; i++) {
					const start = i * chunkSizeWithTag;
					const end = Math.min(start + chunkSizeWithTag, encryptedContent.length);
					const chunk = encryptedContent.slice(start, end);
					const isLastChunk = i === totalChunks - 1;

					console.log(`Processing chunk ${i}:`, {
						start,
						end,
						chunkLength: chunk.length,
						isLastChunk
					});

					const decryptedChunk = wasmInstance.decryptChunk(chunk, isLastChunk);
					if (!decryptedChunk) {
						throw new Error(`Failed to decrypt chunk ${i}`);
					}

					chunks.push(decryptedChunk);

					// Calculate and update progress with proper scaling
					const currentProgress = (i + 1) / totalChunks;
					const scaledProgress = 40 + currentProgress * 50; // Scale between 40% and 90%

					// Ensure UI update happens
					await new Promise((resolve) => {
						requestAnimationFrame(async () => {
							await progressCallback(
								scaledProgress,
								`Dekrypterer... (${Math.round(currentProgress * 100)}%)`
							);
							resolve();
						});
					});
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

	async function updateDownloadProgress(progress: number, action: string) {
		progress = Math.min(Math.max(0, progress), 100);
		if (!Number.isFinite(progress)) {
			progress = 0;
		}

		if (progress >= 100) {
			downloadProgress.style.display = 'none';
			finishedDownloading.style.display = 'block';
			return;
		}

		downloadProgress.style.display = 'block';
		downloadProgressTitle.textContent = action;
		downloadProgressBar.style.width = `${progress}%`;
		downloadProgressText.textContent = `${Math.round(progress)}%`;
	}

	async function downloadFile() {
		const fileId = window.location.pathname.slice(1);
		try {
			const downloadButton = document.getElementById('downloadButton');
			downloadButton.classList.add('hidden');

			await updateDownloadProgress(0, 'Starter nedlasting...');

			// Stream the download with progress
			const response = await fetch(`/api/download/${fileId}`);
			if (!response.ok) throw new Error('Nedlasting feilet');

			const contentLength = +response.headers.get('Content-Length') || 0;
			const reader = response.body.getReader();
			const chunks = [];
			let receivedLength = 0;

			while (true) {
				const { done, value } = await reader.read();

				if (done) break;

				chunks.push(value);
				receivedLength += value.length;

				// Download progress (0% to 40% of total progress)
				// Only calculate progress if contentLength is valid
				if (contentLength > 0) {
					const downloadProgress = Math.min((receivedLength / contentLength) * 40, 40);
					await updateDownloadProgress(
						downloadProgress,
						`Laster ned... (${Math.round((receivedLength / contentLength) * 100)}%)`
					);
				} else {
					await updateDownloadProgress(20, 'Laster ned...');
				}
			}

			// Combine downloaded chunks
			const encryptedData = new Uint8Array(receivedLength);
			let position = 0;
			for (const chunk of chunks) {
				encryptedData.set(chunk, position);
				position += chunk.length;
			}

			await updateDownloadProgress(40, 'Dekrypterer...');

			// Decrypt with progress updates
			const processor = new ChunkedFileProcessor();
			const { decrypted, metadata } = await processor.decryptFile(
				encryptedData,
				encryptionKey,
				async (progress, message) => {
					// Ensure progress is between 40 and 90
					const adjustedProgress = 40 + Math.min(Math.max(0, progress), 100) * 0.5;
					await updateDownloadProgress(adjustedProgress, message);
				}
			);

			await updateDownloadProgress(90, 'Forbereder nedlasting...');

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

			await updateDownloadProgress(100, 'Fullført!');
		} catch (error) {
			console.error('Download error:', error);
			alert('Feil: ' + error.message);
			document.getElementById('downloadButton').classList.remove('hidden');
			document.getElementById('downloadProgress').style.display = 'none';
		}
	}

	function formatFileSize(bytes: number): string {
		const units = ['B', 'KB', 'MB', 'GB'];
		let size = bytes;
		let unitIndex = 0;
		while (size >= 1024 && unitIndex < units.length - 1) {
			size /= 1024;
			unitIndex++;
		}
		return `${size.toFixed(2)} ${units[unitIndex]}`;
	}

	async function updateProgress(progress: number, action: string) {
		if (progress >= 100) {
			progressContainer.style.display = 'none';
			return;
		}

		progressContainer.style.display = 'block';
		progressTitle.textContent = action;
		progressBar.style.width = `${progress}%`;
		progressText.textContent = `${Math.round(progress)}%`;
	}

	function showFileInfo(file: File) {
		fileNameInfo.textContent = `Filnavn: ${file.name}`;
		fileSizeInfo.textContent = `Størrelse: ${formatFileSize(file.size)}`;
		fileInfo.style.display = 'block';
	}

	function copyUrl() {
		urlField.select();
		document.execCommand('copy');
		alert('Lenke kopiert til utklippstavlen!');
	}

	async function uploadFile() {
		if (!fileInput.files?.length) return;
		const file = fileInput.files[0];

		if (file.size > maxFileSize) {
			alert('Filen er for stor! Maksimal størrelse er 1GB');
			return;
		}

		try {
			const processor = new ChunkedFileProcessor();
			const key = encryptionKey || generateKey();

			const { header, encryptedContent } = await processor.encryptFile(file, key);

			await updateProgress(50, 'Laster opp...');
			const formData = new FormData();
			const blob = new Blob([header, encryptedContent]);
			formData.append('file', blob, 'encrypted_container');

			const response = await fetch('/api/upload', {
				method: 'POST',
				body: formData
			});

			if (!response.ok) throw new Error('Opplasting feilet');

			const result = await response.json();
			await updateProgress(100, 'Fullført!');

			const url = `${window.location.origin}/${result.id}${window.location.hash}`;
			window.history.replaceState(null, '', url);
			urlField.value = url;
			urlContainer.style.display = 'block';
		} catch (error) {
			alert('Feil: ' + (error as Error).message);
		}
	}

	function handleUpload() {
		if (!fileInput.files?.length) {
			fileInput.click();
			return;
		}
		uploadFile();
	}

	function generateKey() {
		const wasmInstance = getWasmInstance();
		if (!wasmInstance) return;

		const key = wasmInstance.generateKey();
		window.location.hash = `key=${key}`;
		return key;
	}

	onMount(async () => {
		if (!browser) return;

		try {
			await initWasm();
			isWasmLoaded = true;

			const urlParams = new URLSearchParams(window.location.hash.slice(1));
			encryptionKey = urlParams.get('key') || '';

			if (window.location.pathname.length > 1) {
				uploadSection.style.display = 'none';
				downloadContainer.style.display = 'block';
				await fetchFileMetadata();
			}
		} catch (error) {
			console.error('Failed to initialize application:', error);
			alert('Failed to initialize the application. Please refresh the page.');
		}
		const urlParams = new URLSearchParams(window.location.hash.slice(1));
		encryptionKey = urlParams.get('key') || '';

		if (window.location.pathname.length > 1) {
			uploadSection.style.display = 'none';
			downloadContainer.style.display = 'block';
			await fetchFileMetadata();
		}

		async function ensureWasmLoaded() {
			if (!isWasmLoaded && !isLoading) {
				isLoading = true;
				try {
					await initWasm();
					isWasmLoaded = true;
				} finally {
					isLoading = false;
				}
			}
			const instance = getWasmInstance();
			if (!instance) {
				throw new Error('Failed to initialize WASM');
			}
			return instance;
		}

		async function fetchFileMetadata() {
			if (!(window.location.pathname.length > 1)) return;

			try {
				await ensureWasmLoaded();
				const wasmInstance = getWasmInstance();
				if (!wasmInstance) {
					throw new Error('WASM not initialized');
				}

				const fileId = window.location.pathname.slice(1);
				const response = await fetch(`/api/metadata/${fileId}`);

				if (response.status === 404) {
					downloadContainer.innerHTML = `
                    <div class="error-message">
                        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <circle cx="12" cy="12" r="10"/>
                            <line x1="12" y1="8" x2="12" y2="12"/>
                            <line x1="12" y1="16" x2="12.01" y2="16"/>
                        </svg>
                        <p>Beklager, men filen du leter etter finnes ikke eller har utløpt.
                        Vennligst kontakt avsenderen for å få en ny delingslenke.</p>
                    </div>`;
					return;
				}

				if (!response.ok) throw new Error('Kunne ikke hente filinformasjon');

				const encryptedData = await response.arrayBuffer();
				if (!encryptionKey) {
					if (downloadFileName) {
						downloadFileName.textContent = 'Mangler dekrypteringsnøkkel';
					}
					return;
				}

				try {
					const metadata = await wasmInstance.decryptMetadata(
						encryptionKey,
						new Uint8Array(encryptedData)
					);
					if (downloadFileName) {
						downloadFileName.textContent = `Fil: ${metadata.filename}`;
					}
				} catch (error) {
					if (downloadFileName) {
						downloadFileName.textContent = 'Kunne ikke dekryptere filinformasjon';
					}
				}
			} catch (error) {
				if (downloadFileName) {
					downloadFileName.textContent = 'Kunne ikke hente filinformasjon';
				}
			}
		}

		const cleanup = () => {
			if (fileInput) {
				fileInput.value = '';
			}
			if (selectedFile) {
				selectedFile.textContent = '';
			}
			if (fileInfo) {
				fileInfo.style.display = 'none';
			}
		};

		window.addEventListener('beforeunload', cleanup);
		return () => {
			window.removeEventListener('beforeunload', cleanup);
		};
	});
</script>

<!-- Update these sections in your file -->

<div class="container">
	<div class="upload-section" bind:this={uploadSection}>
		<h1>Vi <span>deler</span> filer sikkert</h1>
		<p class="description">
			Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før de
			lastes opp, og dekrypteres først når mottakeren laster dem ned.
		</p>

		<div class="file-input-container">
			<input type="file" bind:this={fileInput} class="file-input" hidden />
			<button class="button" on:click={handleUpload}>Last opp</button>
			<div class="selected-file" bind:this={selectedFile}></div>
		</div>

		<div class="file-info" bind:this={fileInfo} style="display: none;">
			<div class="progress-title">Filinformasjon</div>
			<div class="file-info-item" bind:this={fileNameInfo}></div>
			<div class="file-info-item" bind:this={fileSizeInfo}></div>
		</div>

		<div class="progress-container" bind:this={progressContainer}>
			<div class="progress-title" bind:this={progressTitle}>Fremgang</div>
			<div class="progress-bar">
				<div class="progress" bind:this={progressBar}></div>
			</div>
			<div class="progress-text" bind:this={progressText}>0%</div>
		</div>

		<div class="url-container" bind:this={urlContainer}>
			<input type="text" class="url-field" bind:this={urlField} readonly />
			<button class="button" on:click={copyUrl}>Kopier lenke</button>
		</div>
	</div>

	<div class="download-container" bind:this={downloadContainer}>
		<div class="progress-title">Last ned fil</div>
		<div class="file-info">
			<div class="file-info-item" bind:this={downloadFileName}>Henter filinformasjon...</div>
		</div>

		<div id="keyPrompt" class="key-prompt" style="display: none;">
			<h2>Dekrypteringsnøkkel kreves</h2>
			<p>
				Du trenger en dekrypteringsnøkkel for å få tilgang til denne filen. Vennligst lim inn
				nøkkelen du har mottatt.
			</p>
			<input type="text" class="key-input" placeholder="Lim inn dekrypteringsnøkkel her" />
			<button class="button" on:click={() => submitKey()}>Fortsett</button>
		</div>

		<div class="success-message" bind:this={finishedDownloading} style="display: none;">
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
		<button class="button" on:click={downloadFile}>Last ned</button>

		<div class="download-progress" bind:this={downloadProgress}>
			<div class="progress-title" bind:this={downloadProgressTitle}>Laster ned...</div>
			<div class="download-progress-bar">
				<div class="download-progress-fill" bind:this={downloadProgressBar}></div>
			</div>
			<div class="download-progress-text" bind:this={downloadProgressText}>0%</div>
		</div>
	</div>
</div>

<style>
	/* Paste the original CSS here */
	:global(:root) {
		--primary-green: #40b87b;
		--dark-text: #1a2634;
		--light-gray: #f5f6f7;
		--border-radius: 8px;
		font-family:
			system-ui,
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			Roboto,
			sans-serif;
	}


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

	h1 span {
		color: var(--primary-green);
	}

	.description {
		font-size: 1.125rem;
		line-height: 1.6;
		max-width: 600px;
		margin-bottom: 2rem;
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
	}

	.button:hover {
		background-color: #359669;
	}

	.button:disabled {
		background-color: #e0e0e0;
		cursor: not-allowed;
	}

	.file-input-container {
		margin-bottom: 2rem;
	}

	.file-input {
		display: none;
	}

	.progress-container {
		display: none;
		margin-top: 2rem;
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 2rem;
	}

	.progress-title {
		font-size: 1.25rem;
		margin-bottom: 1rem;
		font-weight: 500;
	}

	.progress-bar {
		width: 100%;
		height: 8px;
		background-color: #e0e0e0;
		border-radius: 4px;
		overflow: hidden;
	}

	.progress {
		width: 0%;
		height: 100%;
		background-color: var(--primary-green);
		transition: width 0.3s ease-in-out;
	}

	.progress-text {
		margin-top: 0.5rem;
		font-size: 0.875rem;
		color: #666;
	}

	.progress-container {
		display: none;
		margin-top: 2rem;
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 2rem;
	}

	.progress-title {
		font-size: 1.25rem;
		margin-bottom: 1rem;
		font-weight: 500;
	}

	.progress-bar {
		width: 100%;
		height: 8px;
		background-color: #e0e0e0;
		border-radius: 4px;
		overflow: hidden;
	}

	.progress {
		width: 0%;
		height: 100%;
		background-color: var(--primary-green);
		transition: width 0.3s ease-in-out;
	}

	.progress-text {
		margin-top: 0.5rem;
		font-size: 0.875rem;
		color: #666;
	}

	.download-container {
		display: none;
		margin-top: 2rem;
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

	.file-info-item {
		margin: 0.5rem 0;
		color: #666;
	}

	.url-container {
		margin-top: 1rem;
		display: none;
	}

	.url-field {
		width: 100%;
		padding: 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		margin-bottom: 0.5rem;
		font-family: inherit;
	}

	.upload-section {
		display: block;
	}

	.download-progress {
		display: none;
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
