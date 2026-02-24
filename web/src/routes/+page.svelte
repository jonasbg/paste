<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';

	import { browser } from '$app/environment';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { uploadEncryptedFile } from '$lib/services/encryptionService';
	import { generatePassphrase } from '$lib/utils/wordlist';
	import FileInfo from '$lib/components/FileUpload/FileInfo.svelte';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import PassphraseShare from '$lib/components/PassphraseShare/PassphraseShare.svelte';
	import { fade, slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import { configStore } from '$lib/stores/config';

	let fileInput: HTMLInputElement;
	let selectedFile: File | null = null;
	let isUploading = false;
	let uploadProgress = 0;
	let uploadMessage = '';
	let sharePassphrase = '';
	let shareUrl = '';
	let fileSizeError = '';

	// Pre-generated passphrase (fetched on mount)
	let generatedPassphrase = '';

	// Passphrase download
	let passphraseInput = '';
	let passphraseError = '';
	let isDerivingPassphrase = false;

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
		if (sharePassphrase && selectedFile) {
			if (fileInput) fileInput.value = '';
			setTimeout(() => {}, 100);
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
			const { initWasm, getWasmInstance } = await import('$lib/utils/wasm-loader');
			await initWasm();

			// Ensure passphrase is available
			if (!generatedPassphrase) {
				generatedPassphrase = generatePassphrase();
			}

			const wasm = getWasmInstance();
			if (!wasm) throw new Error('WASM not initialized');

			const keySize = parseInt($configStore.data.key_size) || 128;
			const derived = wasm.deriveFromPassphrase(generatedPassphrase, keySize);
			if (derived instanceof Error) throw derived;

			const { fileId, key } = derived as { fileId: string; key: string };

			await uploadEncryptedFile(
				selectedFile,
				key,
				async (progress, message) => {
					uploadProgress = progress;
					uploadMessage = message;
				},
				fileId
			);

			sharePassphrase = generatedPassphrase;
			shareUrl = `${window.location.origin}/${fileId}#key=${key}`;
			cleanupMemoryReferences();
		} catch (error) {
			console.error('Error: ' + (error instanceof Error ? error.message : String(error)));
			fileSizeError = 'Upload Error: ' + (error instanceof Error ? error.message : String(error));
		} finally {
			isUploading = false;
		}
	}

	async function deriveAndNavigate(phrase: string) {
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
	}

	async function handlePassphraseDownload() {
		const phrase = passphraseInput.trim();
		if (!phrase) return;

		isDerivingPassphrase = true;
		passphraseError = '';

		try {
			await deriveAndNavigate(phrase);
		} catch (err) {
			passphraseError = 'Ugyldig delingskode. Prøv igjen.';
			console.error('Derive error:', err);
		} finally {
			isDerivingPassphrase = false;
		}
	}

	function removeFile() {
		selectedFile = null;
		sharePassphrase = '';
		shareUrl = '';
		generatedPassphrase = '';
		uploadProgress = 0;
		uploadMessage = '';
		fileSizeError = '';

		if (fileInput) fileInput.value = '';
		// Generate a new passphrase for the next upload
		generatedPassphrase = generatePassphrase();
		setTimeout(() => {}, 100);
	}

	onMount(async () => {
		if (!browser) return;

		// Wait for config to be loaded
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

		if ($configStore.error) {
			fileSizeError = `Failed to load configuration: ${$configStore.error}`;
		}

		// Handle #passphrase=... in URL (shared via passphrase link)
		if (window.location.hash) {
			const hashParams = new URLSearchParams(window.location.hash.slice(1));
			const passphrase = hashParams.get('passphrase');
			if (passphrase) {
				// Clear hash immediately for security
				history.replaceState(null, '', window.location.pathname);
				try {
					await deriveAndNavigate(passphrase);
					return;
				} catch (err) {
					console.error('Failed to navigate from passphrase link:', err);
				}
			}
		}

		// Generate passphrase client-side so it's ready when user clicks upload
		generatedPassphrase = generatePassphrase();

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

	async function handlePaste(event: ClipboardEvent) {
		const target = event.target as HTMLElement;
		if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') {
			return;
		}

		if (isUploading || sharePassphrase) {
			return;
		}

		const items = event.clipboardData?.items;
		if (!items) return;

		for (let i = 0; i < items.length; i++) {
			const item = items[i];

			if (item.type.startsWith('image/')) {
				event.preventDefault();
				const blob = item.getAsFile();
				if (!blob) continue;

				let filename = 'screenshot.png';
				if (blob.name && blob.name !== 'image.png') {
					filename = blob.name;
				} else {
					const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
					const extension = item.type.split('/')[1] || 'png';
					filename = `screenshot-${timestamp}.${extension}`;
				}

				const file = new File([blob], filename, { type: item.type });
				await processClipboardFile(file);
				return;
			}

			if (item.kind === 'file' && !item.type.startsWith('image/')) {
				event.preventDefault();
				const file = item.getAsFile();
				if (file) {
					await processClipboardFile(file);
					return;
				}
			}

			if (item.type === 'text/plain') {
				event.preventDefault();
				item.getAsString(async (text) => {
					if (!text.trim()) return;

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

			{#if !isUploading && !sharePassphrase}
				<div class="content-grid">
					<!-- Upload column -->
					<div class="upload-column">
						<p class="description">
							Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før
							de lastes opp, og dekrypteres først når mottakeren laster dem ned.
						</p>
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
					</div>

					<!-- Passphrase download column — contracts when file selected -->
					{#if !selectedFile}
						<div
							class="passphrase-panel"
							transition:slide={{ axis: 'x', duration: 350, easing: cubicOut }}
						>
							<div class="vertical-separator">
								<span>eller</span>
							</div>
							<div class="copy-section">
								<h3>Har du en delingskode?</h3>
								<p class="hint">Skriv inn delingskoden du har mottatt for å laste ned filen.</p>
								{#if passphraseError}
									<p class="passphrase-error">{passphraseError}</p>
								{/if}
								<form on:submit|preventDefault={handlePassphraseDownload}>
									<div class="input-group">
										<input
											type="text"
											class="url-field"
											placeholder="Skriv inn delingskoden din"
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
					{/if}
				</div>
			{/if}

			<ProgressBar
				progress={uploadProgress}
				message={uploadMessage}
				isVisible={isUploading}
				fileName={selectedFile?.name}
				fileSize={selectedFile ? FileProcessor.formatFileSize(selectedFile.size) : ''}
			/>
			<PassphraseShare passphrase={sharePassphrase} secureUrl={shareUrl} isVisible={!!sharePassphrase} />
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

	/* Two-column layout */
	.content-grid {
		display: flex;
		align-items: flex-start;
		min-height: 8rem;
		margin-top: 1em;
	}

	.upload-column {
		flex: 1;
		margin-top: 1em;
		min-width: 0;
	}

	/* Passphrase panel: separator + copy section, slides out horizontally */
	.passphrase-panel {
		display: flex;
		align-items: stretch;
		overflow: hidden;
		flex: 1;
		min-width: 0;
	}

	/* Vertical "eller" separator */
	.vertical-separator {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 0 2rem;
		flex-shrink: 0;
		align-self: stretch;
	}

	.vertical-separator::before,
	.vertical-separator::after {
		content: '';
		flex: 1;
		width: 1px;
		background: #e0e0e0;
	}

	.vertical-separator span {
		padding: 0.5rem 0;
		color: #666;
		font-size: 0.875rem;
		flex-shrink: 0;
	}

	.copy-section {
		flex: 1;
		background: #fff;
		padding: 1rem;
		border-radius: var(--border-radius);
		border: 1px solid #e0e0e0;
		min-width: 0;
		margin-top: 2em;
		margin-bottom: 2em;
	}

	.copy-section h3 {
		font-size: 1rem;
		margin: 0 0 0.5rem 0;
		font-weight: 500;
	}

	.hint {
		font-size: 0.875rem;
		color: #666;
		margin-bottom: 0.75rem;
	}

	.input-group {
		display: flex;
		gap: 0.5rem;
	}

	.url-field {
		flex: 1;
		padding: 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		font-family: inherit;
		font-size: 1rem;
		background: #f5f5f5;
		min-width: 0;
	}

	.url-field:focus {
		outline: none;
		border-color: var(--primary-green);
		box-shadow: 0 0 0 2px rgba(64, 184, 123, 0.2);
		background: #fff;
	}

	.passphrase-panel .button {
		max-width: none;
		white-space: nowrap;
	}

	.passphrase-error {
		font-size: 0.875rem;
		color: var(--error-red, #dc3545);
		margin: 0 0 0.75rem 0;
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

	/* Mobile: stack vertically, use slide-y animation implicitly via CSS */
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

		.content-grid {
			flex-direction: column;
		}

		.passphrase-panel {
			flex-direction: column;
			width: 100%;
			overflow: visible;
		}

		.vertical-separator {
			flex-direction: row;
			padding: 1rem 0;
			align-self: auto;
		}

		.vertical-separator::before,
		.vertical-separator::after {
			flex: 1;
			height: 1px;
			width: auto;
		}

		.vertical-separator span {
			padding: 0 1rem;
		}

		.copy-section {
			width: 100%;
		}

		.input-group {
			flex-direction: column;
			gap: 0.75rem;
		}

		.passphrase-panel .button {
			width: 100%;
		}

		.url-field {
			font-size: 16px;
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
			padding: 1rem 1.5rem;
		}
	}
</style>
