<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { initWasm } from '$lib/utils/wasm-loader';
	import { streamDownloadAndDecrypt, fetchMetadata } from '$lib/services/fileService';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import { replaceState } from '$app/navigation';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import { generateHmacToken } from '$lib/utils/hmacUtils';
	import { fly } from 'svelte/transition';

	let encryptionKey: string = '';
	let manualKeyInput: string = '';
	let metadata: any = null;
	let fileSize: string | undefined;
	let downloadProgress = 0;
	let downloadMessage = '';
	let isDownloading = false;
	let downloadError: string | null = null;
	let isDownloadComplete = false;
	let keyError: string | null = null;
	let isLoading = true;
	let deletionError: string | null = null;

	// Smooth progress animation
	let displayProgress = 0;
	let animationFrame: number;
	let downloadStartTime = 0;
	let eta = '';

	function formatEta(seconds: number): string {
		if (!isFinite(seconds) || seconds <= 0 || seconds > 3600) return '';
		if (seconds < 60) return `${Math.ceil(seconds)}s igjen`;
		const mins = Math.floor(seconds / 60);
		const secs = Math.ceil(seconds % 60);
		return `${mins}m${secs > 0 ? ` ${secs}s` : ''} igjen`;
	}

	function animateProgress() {
		const diff = downloadProgress - displayProgress;
		if (Math.abs(diff) < 0.2) {
			displayProgress = downloadProgress;
		} else {
			displayProgress += diff * 0.1;
		}

		if (displayProgress > 2 && displayProgress < 99 && downloadStartTime > 0) {
			const elapsed = (Date.now() - downloadStartTime) / 1000;
			if (elapsed > 0.5) {
				const rate = displayProgress / elapsed;
				eta = formatEta((100 - displayProgress) / rate);
			}
		} else if (displayProgress >= 99) {
			eta = '';
		}

		if (displayProgress !== downloadProgress || isDownloading) {
			animationFrame = requestAnimationFrame(animateProgress);
		}
	}

	$: if (downloadProgress !== displayProgress) {
		if (downloadStartTime === 0 && downloadProgress > 0) {
			downloadStartTime = Date.now();
		}
		if (animationFrame) cancelAnimationFrame(animationFrame);
		animationFrame = requestAnimationFrame(animateProgress);
	}

	$: if (isDownloadComplete) {
		displayProgress = 100;
		eta = '';
	}

	function validateAndExtractKey(input: string): string | null {
		input = input.trim();
		if (input.includes('://')) {
			try {
				const url = new URL(input);
				const hashParams = new URLSearchParams(url.hash.slice(1));
				return hashParams.get('key');
			} catch (error) {
				return null;
			}
		}
		const base64Regex = /^[A-Za-z0-9+/=_-]+$/;
		if (base64Regex.test(input)) return input;
		return null;
	}

	async function getMetadata() {
		try {
			const fileId = $page.params.fileId;
			const hmacToken = await generateHmacToken(fileId, encryptionKey);
			const metadataResponse = await fetchMetadata(fileId, encryptionKey, hmacToken);
			metadata = metadataResponse.metadata;
			fileSize = metadataResponse.size?.toString();
		} catch (error) {
			console.error('Metadata error:', error);
			encryptionKey = '';
			manualKeyInput = '';
			metadata = {
				error:
					'Kunne ikke hente filinformasjon. Sjekk at nøkkelen er riktig, eller at filen ikke er slettet.'
			};
		} finally {
			isLoading = false;
		}
	}

	function setEncryptionKey(key: string) {
		encryptionKey = key;
		if (browser) replaceState('', window.location.pathname);
	}

	async function handleManualKeySubmit() {
		if (!manualKeyInput.trim()) return;
		const key = validateAndExtractKey(manualKeyInput.trim());
		if (key) {
			setEncryptionKey(key);
			await getMetadata();
		} else {
			keyError = 'Ugyldig nøkkel eller URL';
		}
	}

	async function initiateDownload() {
		if (!encryptionKey || isDownloading || !metadata || metadata.error) return;
		isDownloading = true;
		downloadError = null;

		try {
			const fileId = $page.params.fileId;
			const hmacToken = await generateHmacToken(fileId, encryptionKey);

			const { stream, metadata: fileMetadata } = await streamDownloadAndDecrypt(
				fileId,
				encryptionKey,
				hmacToken,
				async (progress, message) => {
					downloadProgress = progress;
					downloadMessage = message;
				}
			);

			const reader = stream.getReader();
			const chunks = [];
			let receivedLength = 0;

			while (true) {
				const { done, value } = await reader.read();
				if (done) break;
				if (value) {
					chunks.push(value);
					receivedLength += value.length;
				}
			}

			if (receivedLength === 0) {
				throw new Error('Kunne ikke dekryptere filen - filen er nå slettet fra serveren');
			}

			const blob = new Blob(chunks, {
				type: fileMetadata.contentType || 'application/octet-stream'
			});

			if (blob.size === 0) {
				throw new Error('Kunne ikke dekryptere filen - filen er nå slettet fra serveren');
			}

			const url = window.URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = fileMetadata.filename;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			window.URL.revokeObjectURL(url);

			try {
				const deleteResponse = await fetch(`/api/delete/${fileId}`, {
					method: 'DELETE',
					headers: {
						'Content-Type': 'application/json',
						'X-HMAC-Token': hmacToken
					}
				});

				encryptionKey = '';
				manualKeyInput = '';

				if (deleteResponse.ok) {
					isDownloadComplete = true;
					deletionError = null;
				} else {
					deletionError = 'Filen ble lastet ned, men kunne ikke slettes fra serveren.';
				}
			} catch (err) {
				deletionError =
					'Filen ble lastet ned, men kunne ikke slettes fra serveren på grunn av en nettverksfeil.';
			}

			if (browser) window.history.replaceState({}, '', '/');
		} catch (error) {
			console.error('Download error:', error);
			downloadError = (error as Error).message;
			downloadProgress = 0;
			downloadMessage = '';
		} finally {
			isDownloading = false;
		}
	}

	onMount(async () => {
		if (!browser) return;

		try {
			await initWasm();

			const fileId = $page.params.fileId;

			const storedKey = sessionStorage.getItem('paste_key_' + fileId);
			if (storedKey) {
				sessionStorage.removeItem('paste_key_' + fileId);
				setEncryptionKey(storedKey);
				await getMetadata();
				return;
			}

			if (window.location.hash) {
				const urlParams = new URLSearchParams(window.location.hash.slice(1));
				const key = urlParams.get('key');
				if (key) {
					const validatedKey = validateAndExtractKey(key);
					if (validatedKey) {
						setEncryptionKey(validatedKey);
						await getMetadata();
					} else {
						keyError = 'Ugyldig nøkkel eller URL';
					}
				}
			}
		} catch (error) {
			console.error('Failed to initialize:', error);
			downloadError = 'Failed to initialize the application';
		} finally {
			isLoading = false;
		}
	});

	$: canDownload = !!(
		metadata &&
		!metadata.error &&
		!isDownloading &&
		!isDownloadComplete &&
		encryptionKey
	);

	$: manualKeyInput, (keyError = null);
</script>

<div class="page-container">
	<div class="container">
		<div class="download-section">
			<h1><a href="/"><span>Sikker</span></a> fildeling</h1>
			<p class="description">
				Velkommen til vår sikre fildelingstjeneste. Her kan du trygt laste ned filer som har blitt
				delt med deg. Alle filer er ende-til-ende-kryptert. Etter vellykket nedlasting blir filen
				automatisk slettet fra våre servere.
			</p>

			{#if isLoading}
				<LoadingSpinner message="" />
			{/if}

			{#if downloadError}
				<ErrorMessage message={downloadError} />
			{:else if metadata?.error}
				<ErrorMessage message={metadata.error} />
			{:else if metadata?.filename}
				<!-- Unified 3-column file row -->
				<div class="file-row" in:fly={{ y: 16, duration: 280 }}>
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
						<div class="file-name">{metadata.filename}</div>
						<div class="file-meta">
							<div class="meta-left">
								{#if fileSize}
									<span class="size">{fileSize}</span>
								{/if}
								{#if eta && isDownloading}
									<span class="dot">·</span>
									<span class="eta">{eta}</span>
								{/if}
							</div>
							{#if isDownloading || isDownloadComplete}
								<span class="pct">{Math.round(displayProgress)}%</span>
							{/if}
						</div>
						{#if isDownloading || isDownloadComplete}
							<div class="progress-track">
								<div
									class="progress-fill"
									class:complete={isDownloadComplete}
									style="width: {displayProgress}%"
								/>
							</div>
						{/if}
					</div>

					<!-- Right: download button → spinner → checkmark -->
					<div class="col-action">
						{#if isDownloadComplete}
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
						{:else if isDownloading}
							<div class="spinner" aria-label="Laster ned..."></div>
						{:else}
							<button
								class="download-btn"
								on:click={initiateDownload}
								disabled={!canDownload}
							>
								Last ned
							</button>
						{/if}
					</div>
				</div>

				{#if isDownloadComplete}
					<p class="deleted-notice" in:fly={{ y: 6, duration: 250 }}>Filen er slettet fra serveren.</p>
				{/if}

				{#if deletionError}
					<ErrorMessage message={deletionError} />
				{/if}
			{/if}

			{#if !encryptionKey && !isDownloadComplete && !isLoading}
				<form class="key-prompt" on:submit|preventDefault={handleManualKeySubmit}>
					<h3>Dekrypteringsnøkkel kreves</h3>
					<p class="hint">
						Lim inn hele lenken du har mottatt, så vil nøkkelen automatisk bli hentet ut.
						Alternativt kan du lime inn dekrypteringsnøkkelen direkte.
					</p>
					<div class="input-group">
						<input
							type="text"
							class="key-input"
							placeholder="Lim inn dekrypteringsnøkkel eller hele URL-en"
							bind:value={manualKeyInput}
							disabled={isDownloading}
						/>
						<button
							type="submit"
							class="button"
							disabled={!manualKeyInput.trim() || isDownloading}
						>
							Dekrypter
						</button>
					</div>
					{#if keyError}
						<p class="key-error">{keyError}</p>
					{/if}
				</form>
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

	.download-section {
		flex: 1;
		display: flex;
		flex-direction: column;
	}

	h1 {
		font-size: 2.5rem;
		font-weight: 500;
		margin-bottom: 0.75rem;
	}

	h1 a {
		text-decoration: none;
		color: inherit;
	}

	h1 span {
		color: var(--primary-green);
		cursor: pointer;
	}

	h1 a:hover span {
		text-decoration: underline;
	}

	.description {
		font-size: 1rem;
		line-height: 1.6;
		color: #555;
		margin-bottom: 1.5rem;
	}

	/* ── Unified 3-column file row ── */
	.file-row {
		display: grid;
		grid-template-columns: 52px 1fr auto;
		align-items: center;
		gap: 1rem;
		background: #fff;
		/* border: 1px solid #e5e7eb; */
		border-radius: 10px;
		padding: 1rem 1.25rem;
		/* box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05); */
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

	.download-btn {
		background-color: var(--primary-green);
		color: white;
		border: none;
		border-radius: 8px;
		padding: 0.625rem 1.25rem;
		font-size: 0.9375rem;
		font-weight: 500;
		font-family: inherit;
		cursor: pointer;
		white-space: nowrap;
		transition: all 0.2s ease;
	}

	.download-btn:hover {
		transform: translateY(-1px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
	}

	.download-btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
		transform: none;
		box-shadow: none;
	}

	/* ── Key prompt ── */
	.key-prompt {
		background: #fff;
		padding: 1.25rem;
		border-radius: 10px;
		border: 1px solid #e5e7eb;
		margin-top: 1.5rem;
	}

	.key-prompt h3 {
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

	.key-input {
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

	.key-input:focus {
		outline: none;
		border-color: var(--primary-green);
		box-shadow: 0 0 0 3px rgba(64, 184, 123, 0.15);
		background: #fff;
	}

	.key-error {
		font-size: 0.8125rem;
		color: var(--error-red, #dc3545);
		margin: 0.5rem 0 0 0;
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

	.deleted-notice {
		font-size: 0.8125rem;
		color: #6b7280;
		margin: 0.625rem 0 0 0;
		text-align: center;
	}

	/* ── Mobile ── */
	@media (max-width: 640px) {
		.container {
			padding: 1rem;
		}

		h1 {
			font-size: 1.75rem;
		}

		.input-group {
			flex-direction: column;
		}

		.button {
			width: 100%;
		}

		.key-input {
			font-size: 16px;
		}
	}
</style>
