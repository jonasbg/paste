<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';

	import { browser } from '$app/environment';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { uploadEncryptedFile } from '$lib/services/encryptionService';
	import { generatePassphrase } from '$lib/utils/wordlist';
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

	// Drag state (scoped to drop zone)
	let dragCounter = 0;
	let isDragging = false;

	$: maxFileSizeLabel = $configStore.data?.max_file_size ?? '–';

	function validateAndSetFile(file: File): boolean {
		if (!$configStore.data) {
			fileSizeError = 'Unable to validate file size: configuration not loaded';
			return false;
		}
		const fileProcessor = new FileProcessor();
		const maxFileSize = fileProcessor.getMaxFileSize();
		if (file.size > maxFileSize) {
			fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(maxFileSize)}.`;
			return false;
		}
		selectedFile = file;
		fileSizeError = '';
		return true;
	}

	async function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files?.length) {
			if (validateAndSetFile(input.files[0])) {
				await handleUpload();
			} else {
				input.value = '';
			}
		}
	}

	function cleanupMemoryReferences() {
		if (sharePassphrase && selectedFile) {
			if (fileInput) fileInput.value = '';
			setTimeout(() => {}, 100);
		}
	}

	async function handleUpload() {
		if (!selectedFile) return;

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
			selectedFile = null;
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
		generatedPassphrase = generatePassphrase();
		setTimeout(() => {}, 100);
	}

	onMount(async () => {
		if (!browser) return;

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

		if (window.location.hash) {
			const hashParams = new URLSearchParams(window.location.hash.slice(1));
			const passphrase = hashParams.get('passphrase');
			if (passphrase) {
				history.replaceState(null, '', window.location.pathname);
				try {
					await deriveAndNavigate(passphrase);
					return;
				} catch (err) {
					console.error('Failed to navigate from passphrase link:', err);
				}
			}
		}

		generatedPassphrase = generatePassphrase();

		window.addEventListener('paste', handlePaste);
	});

	onDestroy(() => {
		if (browser) {
			window.removeEventListener('paste', handlePaste);
		}
	});

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

		if (isUploading || sharePassphrase) return;

		const files = event.dataTransfer?.files;
		if (files?.length) {
			if (validateAndSetFile(files[0])) {
				handleUpload();
			}
		}
	}

	function handleZoneClick() {
		if (!isUploading && !sharePassphrase) {
			fileInput.click();
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
		await handleUpload();
	}
</script>

<div class="page-container">
	<div class="container">
		<div class="upload-section">
			<h1>Vi <span on:click={() => goto('/')}>deler</span> filer sikkert</h1>

			{#if !sharePassphrase}
				<p class="description">
					Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før
					de lastes opp, og dekrypteres først når mottakeren laster dem ned.
				</p>

				{#if fileSizeError}
					<ErrorMessage message={fileSizeError} />
				{/if}

				<!-- Row 1: Drop zone -->
				<div
					class="drop-zone"
					class:dragging={isDragging}
					class:uploading={isUploading}
					on:click={handleZoneClick}
					on:dragenter={handleDragEnter}
					on:dragleave={handleDragLeave}
					on:dragover={handleDragOver}
					on:drop={handleDrop}
					role="button"
					tabindex="0"
					on:keydown={(e) => e.key === 'Enter' && handleZoneClick()}
					aria-label="Velg fil for opplasting"
				>
					<input
						type="file"
						bind:this={fileInput}
						on:change={handleFileSelect}
						class="file-input"
						disabled={isUploading}
					/>

					<div class="drop-icon">
						<svg
							fill="currentColor"
							width="56"
							height="56"
							viewBox="-3.2 -3.2 38.40 38.40"
							version="1.1"
							xmlns="http://www.w3.org/2000/svg"
							stroke="none"
						>
							<path
								d="M0 16v-1.984q0-3.328 2.336-5.664t5.664-2.336q1.024 0 2.176 0.352 0.576-2.752 2.784-4.544t5.056-1.824q3.296 0 5.632 2.368t2.368 5.632q0 0.896-0.32 2.048 0.224-0.032 0.32-0.032 2.464 0 4.224 1.76t1.76 4.224v2.016q0 2.496-1.76 4.224t-4.224 1.76h-0.384q0.288-0.8 0.352-1.44 0.096-1.312-0.32-2.56t-1.408-2.208l-4-4q-1.76-1.792-4.256-1.792t-4.224 1.76l-4 4q-0.96 0.96-1.408 2.24t-0.32 2.592q0.032 0.576 0.256 1.248-2.72-0.608-4.512-2.784t-1.792-5.056zM10.016 22.208q-0.096-0.96 0.576-1.6l4-4q0.608-0.608 1.408-0.608 0.832 0 1.408 0.608l4 4q0.672 0.64 0.608 1.6-0.032 0.288-0.16 0.576-0.224 0.544-0.736 0.896t-1.12 0.32h-1.984v6.016q0 0.832-0.608 1.408t-1.408 0.576-1.408-0.576-0.576-1.408v-6.016h-2.016q-0.608 0-1.088-0.32t-0.768-0.896q-0.096-0.288-0.128-0.576z"
							></path>
						</svg>
					</div>

					<p class="drop-primary">Klikk her for å velge fil — eller dra og slipp for å laste opp</p>
					<p class="drop-secondary">Maksimum filstørrelse {maxFileSizeLabel}</p>
				</div>
			{/if}

			<!-- Row 2: Upload progress -->
			<ProgressBar
				isVisible={isUploading || !!sharePassphrase}
				isComplete={!!sharePassphrase}
				progress={uploadProgress}
				message={uploadMessage}
				fileName={selectedFile?.name ?? ''}
				fileSize={selectedFile ? FileProcessor.formatFileSize(selectedFile.size) : ''}
				fileSizeBytes={selectedFile?.size ?? 0}
			/>

			<PassphraseShare
				passphrase={sharePassphrase}
				secureUrl={shareUrl}
				isVisible={!!sharePassphrase}
			/>

			<!-- Passphrase download panel -->
			{#if !isUploading && !sharePassphrase}
				<div
					class="passphrase-panel"
					transition:slide={{ duration: 300, easing: cubicOut }}
				>
					<div class="horizontal-separator">
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
	</div>
</div>

<style>
	.page-container {
		min-height: 90vh;
		display: flex;
		margin: 0;
		padding: 0;
	}

	.container {
		flex: 1;
		max-width: 860px;
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
		margin-bottom: 0.75rem;
	}

	h1 span {
		color: var(--primary-green);
		cursor: pointer;
	}

	.description {
		font-size: 1rem;
		line-height: 1.6;
		color: #555;
		margin-bottom: 1.5rem;
	}

	.file-input {
		display: none;
	}

	/* ── Drop zone (Row 1) ── */
	.drop-zone {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.6rem;
		border: 2px dashed #d1d5db;
		border-radius: 14px;
		padding: 3rem 2rem;
		cursor: pointer;
		background: #fafafa;
		transition:
			border-color 0.2s ease,
			background 0.2s ease,
			opacity 0.2s ease;
		text-align: center;
		user-select: none;
		-webkit-user-select: none;
	}

	.drop-zone:hover {
		border-color: var(--primary-green);
		background: rgba(64, 184, 123, 0.04);
	}

	.drop-zone:focus-visible {
		outline: 2px solid var(--primary-green);
		outline-offset: 2px;
	}

	.drop-zone.dragging {
		border-color: var(--primary-green);
		border-style: solid;
		background: rgba(64, 184, 123, 0.07);
	}

	.drop-zone.uploading {
		opacity: 0.45;
		pointer-events: none;
		cursor: default;
	}

	.drop-icon {
		color: var(--primary-green);
		margin-bottom: 0.25rem;
		transition: transform 0.2s ease;
	}

	.drop-zone:hover .drop-icon,
	.drop-zone.dragging .drop-icon {
		transform: translateY(-3px);
	}

	.drop-primary {
		font-size: 0.9375rem;
		font-weight: 500;
		color: #374151;
		margin: 0;
	}

	.drop-secondary {
		font-size: 0.8125rem;
		color: #9ca3af;
		margin: 0;
	}

	/* ── Passphrase download panel ── */
	.passphrase-panel {
		margin-top: 1.5rem;
		overflow: hidden;
	}

	.horizontal-separator {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.25rem;
	}

	.horizontal-separator::before,
	.horizontal-separator::after {
		content: '';
		flex: 1;
		height: 1px;
		background: #e5e7eb;
	}

	.horizontal-separator span {
		color: #9ca3af;
		font-size: 0.8125rem;
		flex-shrink: 0;
	}

	.copy-section {
		background: #fff;
		padding: 1.25rem;
		border-radius: 10px;
		border: 1px solid #e5e7eb;
	}

	.copy-section h3 {
		font-size: 0.9375rem;
		margin: 0 0 0.375rem 0;
		font-weight: 600;
		color: #111827;
	}

	.hint {
		font-size: 0.8125rem;
		color: #6b7280;
		margin-bottom: 0.875rem;
	}

	.input-group {
		display: flex;
		gap: 0.5rem;
	}

	.url-field {
		flex: 1;
		padding: 0.625rem 0.875rem;
		border: 1px solid #e5e7eb;
		border-radius: 8px;
		font-family: inherit;
		font-size: 0.9375rem;
		background: #f9fafb;
		min-width: 0;
		color: #111827;
	}

	.url-field:focus {
		outline: none;
		border-color: var(--primary-green);
		box-shadow: 0 0 0 3px rgba(64, 184, 123, 0.15);
		background: #fff;
	}

	.passphrase-error {
		font-size: 0.8125rem;
		color: var(--error-red, #dc3545);
		margin: 0 0 0.75rem 0;
	}

	.button {
		background-color: var(--primary-green);
		color: white;
		border: none;
		border-radius: 8px;
		padding: 0.625rem 1.25rem;
		font-size: 0.9375rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s ease;
		white-space: nowrap;
	}

	.button:hover {
		transform: translateY(-1px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
	}

	.button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
		transform: none;
		box-shadow: none;
	}

	/* ── Mobile ── */
	@media (max-width: 640px) {
		.container {
			padding: 1rem;
		}

		h1 {
			font-size: 1.75rem;
		}

		.drop-zone {
			padding: 2rem 1.25rem;
		}

		.input-group {
			flex-direction: column;
		}

		.button {
			width: 100%;
		}

		.url-field {
			font-size: 16px;
		}
	}
</style>
