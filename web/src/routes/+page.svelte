<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { initWasm } from '$lib/utils/wasm-loader';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { generateKey, uploadEncryptedFile } from '$lib/services/encryptionService';
	import FileInfo from '$lib/components/FileUpload/FileInfo.svelte';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import UrlShare from '$lib/components/UrlShare/UrlShare.svelte';
	import { replaceState } from '$app/navigation';
	import { fade } from 'svelte/transition';

	let fileInput: HTMLInputElement;
	let selectedFile: File | null = null;
	let encryptionKey: string | null = null;
	let isUploading = false;
	let uploadProgress = 0;
	let uploadMessage = '';
	let shareUrl = '';

	$: fileName = selectedFile?.name || '';
	$: fileSize = selectedFile ? FileProcessor.formatFileSize(selectedFile.size) : '';

	async function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files?.length) {
			selectedFile = input.files[0];
		}
	}

	async function handleUpload() {
		if (!selectedFile) {
			fileInput.click();
			return;
		}

		try {
			isUploading = true;
			const key = encryptionKey || generateKey();
			if (!key) throw new Error('Failed to generate encryption key');

			const fileId = await uploadEncryptedFile(selectedFile, key, async (progress, message) => {
				uploadProgress = progress;
				uploadMessage = message;
			});

			shareUrl = `${window.location.origin}/${fileId}#key=${key}`;
			replaceState('', shareUrl);
		} catch (error) {
			console.error('Feil: ' + (error as Error).message);
		} finally {
			isUploading = false;
		}
	}

	onMount(async () => {
		if (!browser) return;
		await initWasm();

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
			selectedFile = files[0];
		}
	}

	function removeFile() {
		selectedFile = null;
		shareUrl = '';
		uploadProgress = 0;
		uploadMessage = '';
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
			<h1>Vi <span>deler</span> filer sikkert</h1>
			<p class="description">
				Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før de
				lastes opp, og dekrypteres først når mottakeren laster dem ned.
			</p>

			{#if !isUploading && !shareUrl}
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
					<button class="button" on:click={handleUpload} disabled={isUploading}>
						{selectedFile ? 'Last opp' : 'Velg en fil'}
					</button>
				</div>
			{/if}

			<ProgressBar progress={uploadProgress} message={uploadMessage} isVisible={isUploading} />
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

	.file-input-container {
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
	}

	.button:hover {
		transform: translateY(-1px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
	}

	.button:disabled {
		opacity: 0.7;
		cursor: not-allowed;
	}
</style>
