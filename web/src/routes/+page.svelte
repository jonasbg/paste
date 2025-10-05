<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { generateKey, uploadEncryptedFile } from '$lib/services/encryptionService';
	import FileInfo from '$lib/components/FileUpload/FileInfo.svelte';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import UrlShare from '$lib/components/UrlShare/UrlShare.svelte';
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
			const { initWasm } = await import('$lib/utils/wasm-loader');
			await initWasm();
			const key = encryptionKey || generateKey();
			if (!key) throw new Error('Failed to generate encryption key');

			const result = await uploadEncryptedFile(selectedFile, key, async (progress, message) => {
				uploadProgress = progress;
				uploadMessage = message;
			});

			shareUrl = `${window.location.origin}/${result.fileId}#key=${key}`;
			replaceState('', shareUrl);
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
			<h1>Vi <span>deler</span> filer sikkert</h1>
			<p class="description">
				Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før de
				lastes opp, og dekrypteres først når mottakeren laster dem ned.
			</p>

			{#if !isUploading && !shareUrl}
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
			{/if}

			<ProgressBar
				progress={uploadProgress}
				message={uploadMessage}
				isVisible={isUploading}
				fileName={selectedFile?.name}
				fileSize={selectedFile ? FileProcessor.formatFileSize(selectedFile.size) : ''}
			/>
			<UrlShare url={shareUrl} isVisible={!!shareUrl} />
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
	}
</style>
