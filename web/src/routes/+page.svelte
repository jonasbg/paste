<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$app/environment';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { uploadEncryptedFile } from '$lib/services/encryptionService';
	import { fetchMetadata, streamDownloadAndDecrypt } from '$lib/services/fileService';
	import { generateHmacToken } from '$lib/utils/hmacUtils';
	import { generatePassphrase } from '$lib/utils/wordlist';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import PassphraseShare from '$lib/components/PassphraseShare/PassphraseShare.svelte';
	import { fade, fly, slide } from 'svelte/transition';
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
	let uploadError = '';
	let generatedPassphrase = '';

	// Passphrase download form
	let passphraseInput = '';
	let passphraseError = '';
	let isDerivingPassphrase = false;

	// Inline passphrase download state
	let passphraseFileId = '';
	let passphraseKey = '';
	let passphraseFileMetadata: any = null;
	let passphraseFileSizeStr = '';
	let isPassphraseDownloading = false;
	let passphraseDownloadProgress = 0;
	let passphraseDisplayProgress = 0;
	let passphraseDownloadComplete = false;
	let passphraseDownloadError = '';
	let passphraseDownloadStartTime = 0;
	let passphraseEta = '';
	let passphraseAnimFrame: number;

	// Drag state
	let dragCounter = 0;
	let isDragging = false;

	$: maxFileSizeLabel = $configStore.data?.max_file_size ?? '–';

	function formatEta(seconds: number): string {
		if (!isFinite(seconds) || seconds <= 0 || seconds > 3600) return '';
		if (seconds < 60) return `${Math.ceil(seconds)}s igjen`;
		const mins = Math.floor(seconds / 60);
		const secs = Math.ceil(seconds % 60);
		return `${mins}m${secs > 0 ? ` ${secs}s` : ''} igjen`;
	}

	function animatePassphraseProgress() {
		const diff = passphraseDownloadProgress - passphraseDisplayProgress;
		if (Math.abs(diff) < 0.2) {
			passphraseDisplayProgress = passphraseDownloadProgress;
		} else {
			passphraseDisplayProgress += diff * 0.1;
		}

		if (
			passphraseDisplayProgress > 2 &&
			passphraseDisplayProgress < 99 &&
			passphraseDownloadStartTime > 0
		) {
			const elapsed = (Date.now() - passphraseDownloadStartTime) / 1000;
			if (elapsed > 0.5) {
				const rate = passphraseDisplayProgress / elapsed;
				passphraseEta = formatEta((100 - passphraseDisplayProgress) / rate);
			}
		} else if (passphraseDisplayProgress >= 99) {
			passphraseEta = '';
		}

		if (passphraseDisplayProgress !== passphraseDownloadProgress || isPassphraseDownloading) {
			passphraseAnimFrame = requestAnimationFrame(animatePassphraseProgress);
		}
	}

	$: if (passphraseDownloadProgress !== passphraseDisplayProgress) {
		if (passphraseDownloadStartTime === 0 && passphraseDownloadProgress > 0) {
			passphraseDownloadStartTime = Date.now();
		}
		if (passphraseAnimFrame) cancelAnimationFrame(passphraseAnimFrame);
		passphraseAnimFrame = requestAnimationFrame(animatePassphraseProgress);
	}

	$: if (passphraseDownloadComplete) {
		passphraseDisplayProgress = 100;
		passphraseEta = '';
	}

	function validateAndSetFile(file: File): boolean {
		if (!$configStore.data) {
			fileSizeError = 'Unable to validate file size: configuration not loaded';
			return false;
		}
		const fp = new FileProcessor();
		const max = fp.getMaxFileSize();
		if (file.size > max) {
			fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(max)}.`;
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
		}
	}

	async function handleUpload() {
		if (!selectedFile) return;
		if (!$configStore.data) {
			fileSizeError = 'Unable to upload: configuration not loaded';
			return;
		}

		const fp = new FileProcessor();
		const max = fp.getMaxFileSize();
		if (selectedFile.size > max) {
			fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(max)}`;
			return;
		}

		try {
			isUploading = true;
			uploadError = '';
			const { initWasm, getWasmInstance } = await import('$lib/utils/wasm-loader');
			await initWasm();

			if (!generatedPassphrase) generatedPassphrase = generatePassphrase();

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
			uploadError = error instanceof Error ? error.message : String(error);
			uploadProgress = 0;
			uploadMessage = '';
			generatedPassphrase = generatePassphrase(); // fresh passphrase → new fileId avoids server-side collision
		} finally {
			isUploading = false;
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

			const hmacToken = await generateHmacToken(fileId, key);
			const response = await fetchMetadata(fileId, key, hmacToken);

			if (response.metadata?.error) {
				throw new Error(response.metadata.error);
			}

			passphraseFileId = fileId;
			passphraseKey = key;
			passphraseFileMetadata = response.metadata;
			passphraseFileSizeStr = response.size?.toString() ?? '';
		} catch (err) {
			passphraseError = 'Ugyldig delingskode eller filen finnes ikke. Prøv igjen.';
			console.error('Passphrase resolve error:', err);
		} finally {
			isDerivingPassphrase = false;
		}
	}

	async function initiatePassphraseDownload() {
		if (!passphraseFileId || !passphraseKey || isPassphraseDownloading) return;

		isPassphraseDownloading = true;
		passphraseDownloadError = '';

		try {
			const hmacToken = await generateHmacToken(passphraseFileId, passphraseKey);

			const { stream, metadata: fileMeta } = await streamDownloadAndDecrypt(
				passphraseFileId,
				passphraseKey,
				hmacToken,
				async (progress) => {
					passphraseDownloadProgress = progress;
				}
			);

			const reader = stream.getReader();
			const chunks: Uint8Array[] = [];
			let receivedLength = 0;

			while (true) {
				const { done, value } = await reader.read();
				if (done) break;
				if (value) {
					chunks.push(value);
					receivedLength += value.length;
				}
			}

			if (receivedLength === 0) throw new Error('Filen er allerede slettet fra serveren');

			const blob = new Blob(chunks, { type: fileMeta.contentType || 'application/octet-stream' });
			if (blob.size === 0) throw new Error('Filen er allerede slettet fra serveren');

			const blobUrl = window.URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = blobUrl;
			a.download = fileMeta.filename;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			window.URL.revokeObjectURL(blobUrl);

			try {
				await fetch(`/api/delete/${passphraseFileId}`, {
					method: 'DELETE',
					headers: { 'Content-Type': 'application/json', 'X-HMAC-Token': hmacToken }
				});
			} catch (err) {
				console.error('Delete error:', err);
			}

			passphraseDownloadComplete = true;
		} catch (error) {
			passphraseDownloadError = (error as Error).message;
			passphraseDownloadProgress = 0;
		} finally {
			isPassphraseDownloading = false;
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
		uploadError = '';
		if (fileInput) fileInput.value = '';
		generatedPassphrase = generatePassphrase();
	}

	function dismissUploadError() {
		uploadError = '';
		selectedFile = null;
		generatedPassphrase = generatePassphrase();
		if (fileInput) fileInput.value = '';
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

		// Handle #passphrase=... in URL — resolve inline and auto-start download
		if (window.location.hash) {
			const hashParams = new URLSearchParams(window.location.hash.slice(1));
			const passphrase = hashParams.get('passphrase');
			if (passphrase) {
				history.replaceState(null, '', window.location.pathname);
				passphraseInput = passphrase;
				await handlePassphraseDownload();
				return;
			}
		}

		generatedPassphrase = generatePassphrase();
		window.addEventListener('paste', handlePaste);
	});

	onDestroy(() => {
		if (browser) window.removeEventListener('paste', handlePaste);
		if (passphraseAnimFrame) cancelAnimationFrame(passphraseAnimFrame);
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
		if (dragCounter === 0) isDragging = false;
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
			if (validateAndSetFile(files[0])) handleUpload();
		}
	}

	function handleZoneClick() {
		if (!isUploading && !sharePassphrase) fileInput.click();
	}

	async function handlePaste(event: ClipboardEvent) {
		const target = event.target as HTMLElement;
		if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') return;
		if (isUploading || sharePassphrase) return;

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
					const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
					filename = `screenshot-${ts}.${item.type.split('/')[1] || 'png'}`;
				}
				await processClipboardFile(new File([blob], filename, { type: item.type }));
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
					const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
					const blob = new Blob([text], { type: 'text/plain' });
					await processClipboardFile(
						new File([blob], `pasted-text-${ts}.txt`, { type: 'text/plain' })
					);
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
		const fp = new FileProcessor();
		const max = fp.getMaxFileSize();
		if (file.size > max) {
			fileSizeError = `Filen er for stor. Maksimal filstørrelse er ${FileProcessor.formatFileSize(max)}.`;
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
			<h1>Vi <span on:click={() => history.pushState(null, '', '/')}>deler</span> filer sikkert</h1>

			{#if !sharePassphrase}
				<p class="description">
					Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før de
					lastes opp, og dekrypteres først når mottakeren laster dem ned.
				</p>

				{#if fileSizeError}
					<ErrorMessage message={fileSizeError} />
				{/if}

				{#if uploadError && selectedFile}
					<div class="retry-row" in:fly={{ y: 6, duration: 200 }}>
						<div class="retry-left">
							<svg
								width="20"
								height="20"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="1.5"
								stroke-linecap="round"
								stroke-linejoin="round"
							>
								<circle cx="12" cy="12" r="10" />
								<line x1="12" y1="8" x2="12" y2="12" />
								<line x1="12" y1="16" x2="12.01" y2="16" />
							</svg>
							<div class="retry-text">
								<span class="retry-filename">{selectedFile.name}</span>
								<span class="retry-errmsg">{uploadError}</span>
							</div>
						</div>
						<div class="retry-actions">
							<button class="btn-retry" on:click={handleUpload}>Prøv igjen</button>
							<button class="btn-dismiss-retry" on:click={dismissUploadError} aria-label="Avbryt">
								<svg
									width="16"
									height="16"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
								>
									<line x1="18" y1="6" x2="6" y2="18" />
									<line x1="6" y1="6" x2="18" y2="18" />
								</svg>
							</button>
						</div>
					</div>
				{/if}

				<!-- Row 1: Drop zone — slides away once a passphrase file is resolved or on upload error -->
				{#if !passphraseFileMetadata && !uploadError}
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
						out:slide={{ duration: 350, easing: cubicOut }}
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

						<p class="drop-primary">
							Klikk her for å velge fil — eller dra og slipp for å laste opp
						</p>
						<p class="drop-secondary">Maksimum filstørrelse {maxFileSizeLabel}</p>
					</div>
				{/if}
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
				<div class="passphrase-panel" transition:slide={{ duration: 300, easing: cubicOut }}>
					<!-- "eller" separator fades away once a file is resolved -->
					{#if !passphraseFileMetadata}
						<div class="horizontal-separator" out:fade={{ duration: 200 }}>
							<span>eller</span>
						</div>
					{/if}

					{#if !passphraseFileMetadata}
						<div class="copy-section" out:slide={{ duration: 250, easing: cubicOut }}>
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
										{isDerivingPassphrase ? 'Laster...' : 'Finn fil'}
									</button>
								</div>
							</form>
						</div>
					{/if}

					{#if passphraseFileMetadata}
						{#if passphraseFileMetadata.error}
							<ErrorMessage message={passphraseFileMetadata.error} />
						{:else}
							<!-- File row below copy-section, no border -->
							<div class="file-row" in:fly={{ y: 12, duration: 260 }}>
								<!-- Left: file icon -->
								<div class="col-icon">
									<svg
										width="32"
										height="32"
										viewBox="0 0 24 24"
										fill="none"
										stroke="currentColor"
										stroke-width="1.5"
										stroke-linecap="round"
										stroke-linejoin="round"
									>
										<path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z" />
										<polyline points="13 2 13 9 20 9" />
									</svg>
								</div>

								<!-- Middle: name · size · eta · progress -->
								<div class="col-info">
									<div class="file-name">{passphraseFileMetadata.filename}</div>
									<div class="file-meta">
										<div class="meta-left">
											{#if passphraseFileSizeStr}
												<span class="size">{passphraseFileSizeStr}</span>
											{/if}
											{#if passphraseEta && isPassphraseDownloading}
												<span class="dot">·</span>
												<span class="eta">{passphraseEta}</span>
											{/if}
										</div>
										{#if isPassphraseDownloading || passphraseDownloadComplete}
											<span class="pct">{Math.round(passphraseDisplayProgress)}%</span>
										{/if}
									</div>
									{#if isPassphraseDownloading || passphraseDownloadComplete}
										<div class="progress-track">
											<div
												class="progress-fill"
												class:complete={passphraseDownloadComplete}
												style="width: {passphraseDisplayProgress}%"
											/>
										</div>
									{/if}
								</div>

								<!-- Right: download button → spinner → checkmark -->
								<div class="col-action">
									{#if passphraseDownloadComplete}
										<div class="checkmark" title="Nedlasting fullført">
											<svg
												fill="currentColor"
												width="28"
												height="28"
												viewBox="-2.4 -2.4 28.80 28.80"
												xmlns="http://www.w3.org/2000/svg"
											>
												<g data-name="Layer 2">
													<g data-name="checkmark-circle-2">
														<rect width="24" height="24" opacity="0"></rect>
														<path
															d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm4.3 7.61l-4.57 6a1 1 0 0 1-.79.39 1 1 0 0 1-.79-.38l-2.44-3.11a1 1 0 0 1 1.58-1.23l1.63 2.08 3.78-5a1 1 0 1 1 1.6 1.22z"
														></path>
													</g>
												</g>
											</svg>
										</div>
									{:else if isPassphraseDownloading}
										<div class="spinner" aria-label="Laster ned..."></div>
									{:else}
										<button class="btn-last-ned" on:click={initiatePassphraseDownload}>
											Last ned
										</button>
									{/if}
								</div>
							</div>

							{#if passphraseDownloadComplete}
								<p class="deleted-notice" in:fly={{ y: 6, duration: 250 }}>
									Filen er slettet fra serveren.
								</p>
							{/if}

							{#if passphraseDownloadError}
								<div class="download-error" in:fly={{ y: 8, duration: 200 }}>
									<ErrorMessage message={passphraseDownloadError} />
								</div>
							{/if}
						{/if}
					{/if}
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

	/* ── Drop zone ── */
	.drop-zone {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.6rem;
		border: 1px solid #d1d5db;
		border-radius: 14px;
		padding: 3rem 2rem;
		cursor: pointer;
		background: #f3f4f6;
		box-shadow:
			inset 0 2px 6px rgba(0, 0, 0, 0.07),
			inset 0 1px 2px rgba(0, 0, 0, 0.04);
		transition:
			border-color 0.2s ease,
			background 0.2s ease,
			box-shadow 0.2s ease,
			opacity 0.2s ease;
		text-align: center;
		user-select: none;
		-webkit-user-select: none;
	}

	.drop-zone:hover {
		border-color: var(--primary-green);
		background: #eef8f3;
		box-shadow:
			inset 0 2px 8px rgba(0, 0, 0, 0.09),
			inset 0 1px 3px rgba(0, 0, 0, 0.05);
	}

	.drop-zone:focus-visible {
		outline: 2px solid var(--primary-green);
		outline-offset: 2px;
	}

	.drop-zone.dragging {
		border-color: var(--primary-green);
		background: #e8f5ee;
		box-shadow:
			inset 0 3px 10px rgba(0, 0, 0, 0.1),
			inset 0 1px 4px rgba(64, 184, 123, 0.15);
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

	/* ── Passphrase panel ── */
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

	/* ── Inline download file row ── */
	.file-row {
		display: grid;
		grid-template-columns: 52px 1fr auto;
		align-items: center;
		gap: 1rem;
		padding: 1.5rem 1rem 0.25rem;
	}

	.col-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		color: #9ca3af;
	}

	.col-info {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		min-width: 0;
	}

	.file-name {
		font-weight: 600;
		font-size: 0.9375rem;
		color: #111827;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.file-meta {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: 0.8125rem;
		color: #6b7280;
	}

	.meta-left {
		display: flex;
		align-items: center;
		gap: 0.3rem;
	}

	.dot {
		color: #d1d5db;
	}

	.pct {
		font-variant-numeric: tabular-nums;
		font-weight: 500;
		color: #374151;
	}

	.progress-track {
		width: 100%;
		height: 5px;
		background: #e5e7eb;
		border-radius: 99px;
		overflow: hidden;
		margin-top: 0.1rem;
	}

	@media (prefers-color-scheme: dark) {
		.progress-track {
			background: #374151;
		}
	}

	.progress-fill {
		height: 100%;
		background: var(--primary-green);
		border-radius: 99px;
		transition: width 0.15s ease;
		background-image: linear-gradient(
			90deg,
			rgba(255, 255, 255, 0) 0%,
			rgba(255, 255, 255, 0.22) 50%,
			rgba(255, 255, 255, 0) 100%
		);
		background-size: 200% 100%;
		animation: shimmer 1.5s linear infinite;
	}

	.progress-fill.complete {
		animation: none;
	}

	@keyframes shimmer {
		to {
			background-position: 200% 0;
		}
	}

	.col-action {
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.checkmark {
		color: var(--primary-green, #22c55e);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.spinner {
		width: 28px;
		height: 28px;
		border: 3px solid #e5e7eb;
		border-top-color: var(--primary-green);
		border-radius: 50%;
		animation: spin 0.75s linear infinite;
		flex-shrink: 0;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.btn-last-ned {
		background-color: var(--primary-green);
		font-weight: bold;
		color: white;
		border: none;
		border-radius: 8px;
		padding: 0.625rem 1.25rem;
		font-size: 0.9375rem;
		font-family: inherit;
		cursor: pointer;
		white-space: nowrap;
		transition: all 0.2s ease;
	}

	.btn-last-ned:hover {
		transform: translateY(-1px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
	}

	.deleted-notice {
		font-size: 0.8125rem;
		color: #6b7280;
		margin: 0.625rem 0 0 0;
		text-align: center;
	}

	.download-error {
		margin-top: 0.75rem;
	}

	/* ── Upload retry row ── */
	.retry-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.875rem 1rem;
		border: 1px solid #fca5a5;
		border-radius: 10px;
		background: #fff5f5;
		margin-bottom: 0.75rem;
	}

	@media (prefers-color-scheme: dark) {
		.retry-row {
			background: #2d1a1a;
			border-color: #7f1d1d;
		}
	}

	.retry-left {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		min-width: 0;
		color: #dc2626;
		flex: 1;
	}

	.retry-left svg {
		flex-shrink: 0;
		color: #dc2626;
	}

	.retry-text {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.retry-filename {
		font-size: 0.875rem;
		font-weight: 600;
		color: #111827;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	@media (prefers-color-scheme: dark) {
		.retry-filename {
			color: #f3f4f6;
		}
	}

	.retry-errmsg {
		font-size: 0.8125rem;
		color: #dc2626;
	}

	.retry-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-shrink: 0;
	}

	.btn-retry {
		background-color: var(--primary-green);
		color: white;
		border: none;
		border-radius: 8px;
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		font-weight: 500;
		font-family: inherit;
		cursor: pointer;
		white-space: nowrap;
		transition: all 0.2s ease;
	}

	.btn-retry:hover {
		transform: translateY(-1px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
	}

	.btn-dismiss-retry {
		background: none;
		border: none;
		padding: 0.25rem;
		cursor: pointer;
		color: #9ca3af;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 4px;
		transition: color 0.15s ease;
	}

	.btn-dismiss-retry:hover {
		color: #374151;
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
