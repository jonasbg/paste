<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { initWasm } from '$lib/utils/wasm-loader';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { generateKey, uploadEncryptedFile } from '$lib/services/encryptionService';
	import FileInfo from '$lib/components/FileUpload/FileInfo.svelte';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import UrlShare from '$lib/components/UrlShare/UrlShare.svelte';

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
			window.history.replaceState(null, '', shareUrl);
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
</script>

<div class="container">
	<div class="upload-section">
		<h1>Vi <span>deler</span> filer sikkert</h1>
		<p class="description">
			Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før de
			lastes opp, og dekrypteres først når mottakeren laster dem ned.
		</p>

		<div class="file-input-container">
			<input
				type="file"
				bind:this={fileInput}
				on:change={handleFileSelect}
				class="file-input"
				hidden
			/>
			<button class="button" on:click={handleUpload} disabled={isUploading}>
				{isUploading ? 'Laster opp...' : 'Last opp'}
			</button>
		</div>

		{#if selectedFile}
			<FileInfo
				fileName={selectedFile.name}
				fileSize={FileProcessor.formatFileSize(selectedFile.size)}
				isVisible={true}
			/>
		{/if}

		<ProgressBar progress={uploadProgress} message={uploadMessage} isVisible={isUploading} />

		<UrlShare url={shareUrl} isVisible={!!shareUrl} />
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

	h1 span {
		color: var(--primary-green);
	}

	.description {
		font-size: 1.125rem;
		line-height: 1.6;
		max-width: 600px;
		margin-bottom: 2rem;
	}

	.file-input-container {
		margin-bottom: 2rem;
	}

	.file-input {
		display: none;
	}
</style>
