<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';

	import { browser } from '$app/environment';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { generateKey, uploadEncryptedFile, generatePassphraseFromServer } from '$lib/services/encryptionService';
	import FileInfo from '$lib/components/FileUpload/FileInfo.svelte';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import UrlShare from '$lib/components/UrlShare/UrlShare.svelte';
	import PassphraseShare from '$lib/components/PassphraseShare/PassphraseShare.svelte';
	import { replaceState } from '$app/navigation';
	import { fade } from 'svelte/transition';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import { configStore } from '$lib/stores/config';

	let fileInput: HTMLInputElement;
	let selectedFile: File | null = null;
	let encryptionKey: string | null = null;
	let isUploading = false;
	let uploadProgress = 0;
	let uploadMessage = '';
	let shareUrl = '';
	let fileSizeError = '';

	// Passphrase upload mode
	let usePassphrase = false;
	let generatedPassphrase = '';
	let sharePassphrase = '';

	// Passphrase download section
	let passphraseInput = '';
	let passphraseError = '';
	let isDerivingPassphrase = false;

	async function togglePassphrase() {
		usePassphrase = !usePassphrase;
		if (usePassphrase && !generatedPassphrase) {
			try {
				generatedPassphrase = await generatePassphraseFromServer();
			} catch {
				usePassphrase = false;
				fileSizeError = 'Kunne ikke generere løsenord. Prøv igjen.';
			}
		}
	}

	async function handlePassphraseDownload() {
		const phrase = passphraseInput.trim();
		if (!phrase) return;

		isDerivingPassphrase = true;
		passphraseError = '';

		try {
			const { initWasm, getWasmInstance } = await import('$lib/utils/wasm-loader');
			await initWasm();
			const wasm = getWasmInstance();
			if (!wasm) throw new Error('WASM not initialized');

			const config = $configStore.data;
			const keySize = config ? parseInt(config.key_size) : 128;

			const result = wasm.deriveFromPassphrase(phrase, keySize);
			if (result instanceof Error) throw result;

			const { fileId, key } = result as { fileId: string; key: string };
			sessionStorage.setItem('paste_key_' + fileId, key);
			goto('/' + fileId);
		} catch (err) {
			passphraseError = 'Ugyldig løsenord eller feil. Prøv igjen.';
			console.error('Passphrase derive error:', err);
		} finally {
			isDerivingPassphrase = false;
		}
	}

	async function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files?.length) {
			const file = input.files[0];
			if (!$configStore.data) {
				fileSizeError = 'Unable to validate file size: configuration not loaded';
				selectedFile = null;
				input.value = '';
				return;
			}

			const fileProcessor = new FileProcessor();
			const maxFileSize = fileProcessor.getMaxFileSize();

			if (file.size > maxFileSize) {
				fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(maxFileSize)}.`;
				selectedFile = null;
				input.value = '';
				return;
			}
			selectedFile = file;
			fileSizeError = '';
		}
	}

	function cleanupMemoryReferences() {
		// Release file input references but keep selectedFile for display
		if (shareUrl && selectedFile) {
			const tempFileRef = selectedFile;

			// Clear the file input value to release browser's reference to the file
			if (fileInput) {
				fileInput.value = '';
			}

			// Suggest browser to garbage collect
			setTimeout(() => {
				// This empty timeout can help trigger GC in some browsers
				console.log('Cleanup completed for file:', tempFileRef.name);
			}, 100);
		}
	}

	async function handleUpload() {
		if (!selectedFile) {
			fileInput.click();
			return;
		}

		if (!$configStore.data) {
			fileSizeError = 'Unable to upload: configuration not loaded';
			return;
		}

		const fileProcessor = new FileProcessor();
		const maxFileSize = fileProcessor.getMaxFileSize();

		if (selectedFile.size > maxFileSize) {
			fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(maxFileSize)}`;
			return;
		}

		try {
			isUploading = true;
			// Lazy load WASM runtime just-in-time before generating key / encrypting
			const { initWasm, getWasmInstance } = await import('$lib/utils/wasm-loader');
			await initWasm();

			if (usePassphrase) {
				// Passphrase mode: derive fileId + key from passphrase
				const phrase = generatedPassphrase || await generatePassphraseFromServer();
				if (!generatedPassphrase) generatedPassphrase = phrase;

				const wasm = getWasmInstance();
				if (!wasm) throw new Error('WASM not initialized');

				const keySize = parseInt($configStore.data.key_size) || 128;
				const derived = wasm.deriveFromPassphrase(phrase, keySize);
				if (derived instanceof Error) throw derived;

				const { fileId, key } = derived as { fileId: string; key: string };

				await uploadEncryptedFile(selectedFile, key, async (progress, message) => {
					uploadProgress = progress;
					uploadMessage = message;
				}, fileId);

				sharePassphrase = phrase;
				shareUrl = '';
			} else {
				// Normal mode: random key
				const key = encryptionKey || generateKey();
				if (!key) throw new Error('Failed to generate encryption key');

				const result = await uploadEncryptedFile(selectedFile, key, async (progress, message) => {
					uploadProgress = progress;
					uploadMessage = message;
				});

				shareUrl = `${window.location.origin}/${result.fileId}#key=${key}`;
				replaceState('', shareUrl);
				sharePassphrase = '';
			}

			cleanupMemoryReferences();
		} catch (error) {
			console.error('Error: ' + (error instanceof Error ? error.message : String(error)));
			fileSizeError = 'Upload Error: ' + (error instanceof Error ? error.message : String(error));
		} finally {
			isUploading = false;
		}
	}

	onMount(async () => {
		if (!browser) return;

		// Wait for config to be loaded if it's still loading
		if ($configStore.loading) {
			await new Promise<void>((resolve) => {
				const unsubscribe = configStore.subscribe((state) => {
					if (!state.loading) {
						unsubscribe();
						resolve();
					}
				});
			});
		}

		// Check for config errors
		if ($configStore.error) {
			fileSizeError = `Failed to load configuration: ${$configStore.error}`;
		}

		const urlParams = new URLSearchParams(window.location.hash.slice(1));
		encryptionKey = urlParams.get('key');

		// Add paste event listener
		window.addEventListener('paste', handlePaste);
	});

	onDestroy(() => {
		if (browser) {
			window.removeEventListener('paste', handlePaste);
		}
	});

	let dragCounter = 0;
	let isDragging = false;

	function handleDragEnter(event: DragEvent) {
		event.preventDefault();
		event.stopPropagation();
		dragCounter++;
		isDragging = true;
	}

	function handleDragLeave(event: DragEvent) {
		event.preventDefault();
		event.stopPropagation();
		dragCounter--;
		if (dragCounter === 0) {
			isDragging = false;
		}
	}

	function handleDragOver(event: DragEvent) {
		event.preventDefault();
		event.stopPropagation();
	}

	function handleDrop(event: DragEvent) {
		event.preventDefault();
		event.stopPropagation();
		isDragging = false;
		dragCounter = 0;

		const files = event.dataTransfer?.files;
		if (files?.length) {
			const file = files[0];

			if (!$configStore.data) {
				fileSizeError = 'Unable to validate file size: configuration not loaded';
				selectedFile = null;
				return;
			}

			const fileProcessor = new FileProcessor();
			const maxFileSize = fileProcessor.getMaxFileSize();

			if (file.size > maxFileSize) {
				fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(maxFileSize)}.`;
				selectedFile = null;
				return;
			}
			selectedFile = file;
			fileSizeError = '';
		}
	}

	function removeFile() {
		selectedFile = null;
		shareUrl = '';
		sharePassphrase = '';
		uploadProgress = 0;
		uploadMessage = '';
		fileSizeError = '';

		// Clear the file input value
		if (fileInput) {
			fileInput.value = '';
		}

		// Force a small delay to help with garbage collection
		setTimeout(() => {}, 100);
	}

	async function handlePaste(event: ClipboardEvent) {
		// Don't interfere if user is pasting in an input/textarea
		const target = event.target as HTMLElement;
		if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') {
			return;
		}

		// Don't process paste if we're already uploading or have uploaded
		if (isUploading || shareUrl) {
			return;
		}

		const items = event.clipboardData?.items;
		if (!items) return;

		// Process clipboard items
		for (let i = 0; i < items.length; i++) {
			const item = items[i];

			// Handle images (screenshots, copied images)
			if (item.type.startsWith('image/')) {
				event.preventDefault();
				const blob = item.getAsFile();
				if (!blob) continue;

				// Try to extract filename from clipboard if available
				// Some browsers/OS provide filename metadata
				let filename = 'screenshot.png';

				// Check if the blob has a name property (Firefox on some systems)
				if (blob.name && blob.name !== 'image.png') {
					filename = blob.name;
				} else {
					// Generate filename with timestamp for screenshots
					const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
					const extension = item.type.split('/')[1] || 'png';
					filename = `screenshot-${timestamp}.${extension}`;
				}

				// Create a new File object with the correct filename
				const file = new File([blob], filename, { type: item.type });
				await processClipboardFile(file);
				return;
			}

			// Handle files
			if (item.kind === 'file' && !item.type.startsWith('image/')) {
				event.preventDefault();
				const file = item.getAsFile();
				if (file) {
					await processClipboardFile(file);
					return;
				}
			}

			// Handle text (convert to .txt file)
			if (item.type === 'text/plain') {
				event.preventDefault();
				item.getAsString(async (text) => {
					if (!text.trim()) return;

					// Create a .txt file from the pasted text
					const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
					const filename = `pasted-text-${timestamp}.txt`;
					const blob = new Blob([text], { type: 'text/plain' });
					const file = new File([blob], filename, { type: 'text/plain' });

					await processClipboardFile(file);
				});
				return;
			}
		}
	}

	async function processClipboardFile(file: File) {
		if (!$configStore.data) {
			fileSizeError = 'Unable to validate file size: configuration not loaded';
			return;
		}

		const fileProcessor = new FileProcessor();
		const maxFileSize = fileProcessor.getMaxFileSize();

		if (file.size > maxFileSize) {
			fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(maxFileSize)}.`;
			return;
		}

		selectedFile = file;
		fileSizeError = '';
	}
</script>

<!-- Rest of the template remains the same -->
<div
	class="page-container"
	on:dragenter={handleDragEnter}
	on:dragleave={handleDragLeave}
	on:dragover={handleDragOver}
	on:drop={handleDrop}
>
	<div class="container">
		<div class="upload-section">
			<h1>Vi <span on:click={() => goto('/')}>deler</span> filer sikkert</h1>
			<p class="description">
				Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før de
				lastes opp, og dekrypteres først når mottakeren laster dem ned.
			</p>

			{#if !isUploading && !shareUrl && !sharePassphrase}
				{#if fileSizeError}
					<ErrorMessage message={fileSizeError} />
				{/if}
				{#if selectedFile}
					<FileInfo
						fileName={selectedFile.name}
						fileSize={FileProcessor.formatFileSize(selectedFile.size)}
						isVisible={true}
						onRemove={removeFile}
					/>
				{/if}
				<div class="file-input-container">
					<input
						type="file"
						bind:this={fileInput}
						on:change={handleFileSelect}
						class="file-input"
						hidden
					/>
					<button
						class="button"
						on:click={handleUpload}
						disabled={isUploading || $configStore.loading}
					>
						{selectedFile ? 'Last opp' : 'Velg en fil'}
					</button>
				</div>
				<label class="passphrase-toggle">
					<input type="checkbox" checked={usePassphrase} on:change={togglePassphrase} />
					Del via løsenord
				</label>
				{#if usePassphrase && generatedPassphrase}
					<div class="passphrase-preview">
						<span class="passphrase-label">Løsenord:</span>
						<span class="passphrase-value">{generatedPassphrase}</span>
					</div>
				{/if}
			{/if}

			<ProgressBar
				progress={uploadProgress}
				message={uploadMessage}
				isVisible={isUploading}
				fileName={selectedFile?.name}
				fileSize={selectedFile ? FileProcessor.formatFileSize(selectedFile.size) : ''}
			/>
			<UrlShare url={shareUrl} isVisible={!!shareUrl} />
			<PassphraseShare passphrase={sharePassphrase} isVisible={!!sharePassphrase} />
		</div>
	</div>
</div>

<div class="passphrase-download-section">
	<div class="container">
		<div class="passphrase-download-inner">
			<h2>Har du et løsenord?</h2>
			<p class="description">Skriv inn løsenordet du har mottatt for å laste ned filen.</p>
			{#if passphraseError}
				<ErrorMessage message={passphraseError} />
			{/if}
			<form class="passphrase-form" on:submit|preventDefault={handlePassphraseDownload}>
				<div class="input-group">
					<input
						type="text"
						class="passphrase-input"
						placeholder="Skriv inn løsenordet ditt"
						bind:value={passphraseInput}
						disabled={isDerivingPassphrase}
					/>
					<button
						type="submit"
						class="button"
						disabled={!passphraseInput.trim() || isDerivingPassphrase}
					>
						{isDerivingPassphrase ? 'Laster...' : 'Last ned'}
					</button>
				</div>
			</form>
		</div>
	</div>
</div>

{#if isDragging}
	<div
		class="drop-overlay"
		transition:fade={{ duration: 200 }}
		on:dragenter={handleDragEnter}
		on:dragleave={handleDragLeave}
		on:dragover={handleDragOver}
		on:drop={handleDrop}
	>
		<div class="drop-overlay-content">
			<div class="drop-icon">
				<svg
					width="48"
					height="48"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
					<polyline points="17 8 12 3 7 8" />
					<line x1="12" y1="3" x2="12" y2="15" />
				</svg>
			</div>
			<div class="drop-text">Slipp filen her for å laste opp</div>
		</div>
	</div>
{/if}

<style>
	.page-container {
		min-height: 90vh;
		display: flex;
		/* flex-direction: column; */
		margin: 0;
		padding: 0;
	}

	.container {
		flex: 1;
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
		display: flex;
		flex-direction: column;
	}

	.upload-section {
		flex: 1;
		display: flex;
		flex-direction: column;
	}

	h1 {
		font-size: 2.5rem;
		font-weight: 500;
		margin-bottom: 1.5rem;
	}

	h1 span {
		color: var(--primary-green);
		cursor: pointer;
	}

	.description {
		font-size: 1.125rem;
		line-height: 1.6;
		max-width: 600px;
		margin-bottom: 1.25rem;
	}

	.file-input {
		display: none;
	}

	.drop-overlay {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background-color: rgba(255, 255, 255, 0.95);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
	}

	.drop-overlay-content {
		text-align: center;
		padding: 2rem;
		border: 3px dashed var(--primary-green);
		border-radius: 16px;
		background-color: rgba(var(--primary-green-rgb), 0.05);
		min-width: 300px;
		min-height: 200px;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 1rem;
	}

	.drop-icon {
		color: var(--primary-green);
		animation: bounce 1s infinite;
	}

	.drop-text {
		font-size: 1.25rem;
		color: var(--primary-green);
		font-weight: 500;
	}

	@keyframes bounce {
		0%,
		100% {
			transform: translateY(0);
		}
		50% {
			transform: translateY(-10px);
		}
	}

	.button {
		background-color: var(--primary-green);
		color: white;
		border: none;
		border-radius: 6px;
		padding: 0.75rem 1.5rem;
		font-size: 1rem;
		cursor: pointer;
		transition: all 0.2s ease;
		max-width: 8em;
	}

	.button:hover {
		transform: translateY(-1px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
	}

	.button:disabled {
		opacity: 0.7;
		cursor: not-allowed;
	}

	.passphrase-toggle {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.75rem;
		font-size: 0.875rem;
		color: #555;
		cursor: pointer;
		user-select: none;
	}

	.passphrase-toggle input[type='checkbox'] {
		cursor: pointer;
		accent-color: var(--primary-green);
	}

	.passphrase-preview {
		margin-top: 0.5rem;
		padding: 0.5rem 0.75rem;
		background: rgba(var(--primary-green-rgb), 0.05);
		border: 1px solid rgba(var(--primary-green-rgb), 0.2);
		border-radius: var(--border-radius);
		font-size: 0.875rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.passphrase-label {
		color: #666;
		flex-shrink: 0;
	}

	.passphrase-value {
		font-weight: 500;
		color: #222;
	}

	.passphrase-download-section {
		border-top: 1px solid #e0e0e0;
		padding: 2rem 0;
		background: #fafafa;
	}

	.passphrase-download-inner {
		max-width: 600px;
	}

	.passphrase-download-inner h2 {
		font-size: 1.5rem;
		font-weight: 500;
		margin-bottom: 0.5rem;
	}

	.passphrase-form {
		margin-top: 1rem;
	}

	.input-group {
		display: flex;
		gap: 0.5rem;
	}

	.passphrase-input {
		flex: 1;
		padding: 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		font-family: inherit;
		font-size: 1rem;
		background: #fff;
	}

	.passphrase-input:focus {
		outline: none;
		border-color: var(--primary-green);
		box-shadow: 0 0 0 2px rgba(64, 184, 123, 0.2);
	}

	.passphrase-download-section .button {
		max-width: none;
		white-space: nowrap;
	}

	/* Responsive styles */
	@media (max-width: 768px) {
		.container {
			padding: 1rem;
		}

		h1 {
			font-size: 2rem;
			margin-bottom: 1rem;
		}

		.description {
			font-size: 1rem;
			margin-bottom: 1rem;
		}

		.drop-overlay-content {
			margin: 1rem;
			padding: 1.5rem;
			min-width: auto;
			min-height: 150px;
		}

		.drop-text {
			font-size: 1rem;
		}

		.button {
			/* width: 100%; */
			padding: 1rem 1.5rem;
		}

		.input-group {
			flex-direction: column;
			gap: 0.75rem;
		}

		.passphrase-download-section .button {
			width: 100%;
			max-width: none;
		}

		.passphrase-input {
			font-size: 16px;
		}
	}
</style>
