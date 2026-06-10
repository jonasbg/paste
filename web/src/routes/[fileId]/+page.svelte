<script lang="ts">
	import { run, preventDefault } from 'svelte/legacy';

	import { onDestroy, onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { t, tr } from '$lib/i18n';
	import { initWasm } from '$lib/utils/wasm-loader';
	import {
		downloadAndDecryptFile,
		streamDownloadAndDecrypt,
		fetchMetadata
	} from '$lib/services/fileService';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import { replaceState } from '$app/navigation';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import { generateHmacToken } from '$lib/utils/hmacUtils';
	import { renderTextPreview } from '$lib/utils/textPreview';
	import { isTextBased } from '$lib/utils/mimeType';
	import { fly } from 'svelte/transition';

	const TEXT_PREVIEW_MAX_BYTES = 1024 * 1024;
	const TEXT_PREVIEW_MAX_CHARS = 120_000;
	const IMAGE_PREVIEW_MAX_BYTES = 10 * 1024 * 1024;
	const TEXT_PREVIEW_EXTENSIONS = new Set([
		'txt',
		'md',
		'markdown',
		'json',
		'jsonl',
		'log',
		'csv',
		'tsv',
		'xml',
		'yaml',
		'yml',
		'toml',
		'ini',
		'conf',
		'cfg',
		'env',
		'sh',
		'bash',
		'zsh',
		'fish',
		'ps1',
		'js',
		'ts',
		'jsx',
		'tsx',
		'css',
		'scss',
		'html',
		'htm',
		'svelte',
		'py',
		'rb',
		'php',
		'java',
		'go',
		'rs',
		'c',
		'cc',
		'cpp',
		'h',
		'hpp',
		'sql',
		'kt',
		'kts',
		'swift',
		'dart',
		'lua',
		'r',
		'rmd',
		'vue',
		'astro',
		'scala',
		'ex',
		'exs',
		'hs',
		'zig',
		'diff',
		'patch',
		'lock',
		'gradle',
		'pl',
		'pm',
		'properties',
		'rtf'
	]);
	const IMAGE_PREVIEW_EXTENSIONS = new Set([
		'png',
		'jpg',
		'jpeg',
		'gif',
		'webp',
		'bmp',
		'avif',
		'svg'
	]);
	const IMAGE_MIME_TYPES: Record<string, string> = {
		png: 'image/png',
		jpg: 'image/jpeg',
		jpeg: 'image/jpeg',
		gif: 'image/gif',
		webp: 'image/webp',
		bmp: 'image/bmp',
		avif: 'image/avif',
		svg: 'image/svg+xml'
	};

	type FileMetadata = {
		filename?: string;
		contentType?: string;
		size?: number;
		error?: string;
	};

	let encryptionKey: string = $state('');
	let manualKeyInput: string = $state('');
	let metadata = $state<FileMetadata | null>(null);
	let fileSize: string | undefined = $state();
	let downloadProgress = $state(0);
	let downloadMessage = '';
	let isDownloading = $state(false);
	let downloadError: string | null = $state(null);
	let isDownloadComplete = $state(false);
	let keyError: string | null = $state(null);
	let isLoading = $state(true);
	let deletionError: string | null = $state(null);
	let textPreview: string | null = $state(null);
	let textPreviewHtml = $state('');
	let textPreviewMode: 'pre' | 'table' = $state('pre');
	let textPreviewError: string | null = $state(null);
	let isLoadingTextPreview = $state(false);
	let isTextPreviewTruncated = $state(false);
	let copyState: 'idle' | 'copied' | 'error' = $state('idle');
	let copyResetTimer: ReturnType<typeof setTimeout> | null = null;
	let imagePreviewUrl: string | null = $state(null);
	let imagePreviewError: string | null = $state(null);
	let isLoadingImagePreview = $state(false);
	let previewRequestId = 0;

	// Smooth progress animation
	let displayProgress = $state(0);
	let animationFrame: number | undefined = $state();
	let downloadStartTime = $state(0);
	let eta = $state('');

	function formatEta(seconds: number): string {
		if (!isFinite(seconds) || seconds <= 0 || seconds > 3600) return '';
		if (seconds < 60) return `${Math.ceil(seconds)}s ${tr('common.remaining')}`;
		const mins = Math.floor(seconds / 60);
		const secs = Math.ceil(seconds % 60);
		return `${mins}m${secs > 0 ? ` ${secs}s` : ''} ${tr('common.remaining')}`;
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

	run(() => {
		if (downloadProgress !== displayProgress) {
			if (downloadStartTime === 0 && downloadProgress > 0) {
				downloadStartTime = Date.now();
			}
			if (animationFrame) cancelAnimationFrame(animationFrame);
			animationFrame = requestAnimationFrame(animateProgress);
		}
	});

	run(() => {
		if (isDownloadComplete) {
			displayProgress = 100;
			eta = '';
		}
	});

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

	function getCurrentFileId(): string {
		const fileId = $page.params.fileId;
		if (!fileId) {
			throw new Error('Missing file ID');
		}
		return fileId;
	}

	function formatPreviewLimit(bytes: number): string {
		if (bytes >= 1024 * 1024) {
			return `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
		}

		if (bytes >= 1024) {
			return `${(bytes / 1024).toFixed(0)} KB`;
		}

		return `${bytes} B`;
	}

	function getFileExtension(filename: string | undefined): string {
		if (!filename || !filename.includes('.')) return '';
		return filename.split('.').pop()?.toLowerCase() || '';
	}

	function isTextPreviewable(fileMetadata: FileMetadata | null): boolean {
		if (!fileMetadata?.filename) return false;

		// Trust the server-stored contentType (normalized at upload time)
		const contentType = fileMetadata.contentType?.toLowerCase() || '';
		if (contentType && isTextBased(contentType)) return true;

		// Extension fallback for files uploaded before MIME normalization
		return TEXT_PREVIEW_EXTENSIONS.has(getFileExtension(fileMetadata.filename));
	}

	function isImagePreviewable(fileMetadata: FileMetadata | null): boolean {
		if (!fileMetadata?.filename) return false;

		const contentType = fileMetadata.contentType?.toLowerCase() || '';
		if (contentType.startsWith('image/')) return true;

		return IMAGE_PREVIEW_EXTENSIONS.has(getFileExtension(fileMetadata.filename));
	}

	function resolveImagePreviewType(fileMetadata: FileMetadata): string | null {
		const contentType = fileMetadata.contentType?.toLowerCase() || '';
		if (contentType.startsWith('image/')) {
			return contentType;
		}

		return IMAGE_MIME_TYPES[getFileExtension(fileMetadata.filename)] || null;
	}

	function getUnavailableDownloadMessage(error: unknown): string | null {
		const message = error instanceof Error ? error.message : String(error);
		if (message.includes('Failed to download file') || message.includes('Invalid access token')) {
			return 'Filen finnes ikke eller har allerede blitt lastet ned.';
		}
		return null;
	}

	function fitViewport(
		node: HTMLElement,
		options: { reserveSelector?: string; bottomGap?: number; minHeight?: number } = {}
	) {
		let frame = 0;
		let resizeObserver: ResizeObserver | null = null;
		let currentOptions = options;

		const getOuterHeight = (element: Element) => {
			const styles = window.getComputedStyle(element as HTMLElement);
			const marginTop = parseFloat(styles.marginTop) || 0;
			const marginBottom = parseFloat(styles.marginBottom) || 0;
			return (element as HTMLElement).getBoundingClientRect().height + marginTop + marginBottom;
		};

		const update = () => {
			frame = 0;
			const rect = node.getBoundingClientRect();
			const viewportHeight = window.visualViewport?.height || window.innerHeight;
			const reserveHeight = currentOptions.reserveSelector
				? Array.from(
						node.closest('.download-section')?.querySelectorAll(currentOptions.reserveSelector) ??
							[]
					).reduce((total, element) => total + getOuterHeight(element), 0)
				: 0;
			const bottomGap = currentOptions.bottomGap ?? 24;
			const minHeight = currentOptions.minHeight ?? 120;
			const availableHeight = Math.max(
				minHeight,
				Math.floor(viewportHeight - rect.top - reserveHeight - bottomGap)
			);
			node.style.maxHeight = `${availableHeight}px`;
		};

		const scheduleUpdate = () => {
			if (frame) cancelAnimationFrame(frame);
			frame = requestAnimationFrame(update);
		};

		scheduleUpdate();
		window.addEventListener('resize', scheduleUpdate);
		window.visualViewport?.addEventListener('resize', scheduleUpdate);

		if ('ResizeObserver' in window) {
			resizeObserver = new ResizeObserver(scheduleUpdate);
			resizeObserver.observe(document.body);
		}

		return {
			update(nextOptions: typeof currentOptions = currentOptions) {
				currentOptions = nextOptions;
				scheduleUpdate();
			},
			destroy() {
				if (frame) cancelAnimationFrame(frame);
				window.removeEventListener('resize', scheduleUpdate);
				window.visualViewport?.removeEventListener('resize', scheduleUpdate);
				resizeObserver?.disconnect();
			}
		};
	}

	function resetTextPreview() {
		textPreview = null;
		textPreviewHtml = '';
		textPreviewMode = 'pre';
		textPreviewError = null;
		isLoadingTextPreview = false;
		isTextPreviewTruncated = false;
		if (copyResetTimer) {
			clearTimeout(copyResetTimer);
			copyResetTimer = null;
		}
		copyState = 'idle';
	}

	async function copyTextPreview() {
		if (textPreview === null) return;
		try {
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(textPreview);
				copyState = 'copied';
			} else {
				// Fallback: use a temporary textarea for older browsers / insecure contexts
				const textarea = document.createElement('textarea');
				textarea.value = textPreview;
				textarea.style.position = 'fixed';
				textarea.style.left = '-9999px';
				textarea.style.top = '-9999px';
				document.body.appendChild(textarea);
				textarea.focus();
				textarea.select();
				const success = document.execCommand('copy');
				document.body.removeChild(textarea);
				copyState = success ? 'copied' : 'error';
			}
		} catch {
			copyState = 'error';
		}
		if (copyResetTimer) clearTimeout(copyResetTimer);
		copyResetTimer = setTimeout(() => {
			copyState = 'idle';
			copyResetTimer = null;
		}, 1800);
	}

	function resetImagePreview() {
		if (imagePreviewUrl && browser) {
			URL.revokeObjectURL(imagePreviewUrl);
		}

		imagePreviewUrl = null;
		imagePreviewError = null;
		isLoadingImagePreview = false;
	}

	function resetPreviews(): number {
		previewRequestId += 1;
		resetTextPreview();
		resetImagePreview();
		return previewRequestId;
	}

	async function loadTextPreview(
		fileId: string,
		key: string,
		token: string,
		fileMetadata: FileMetadata,
		requestId: number
	) {
		if (!isTextPreviewable(fileMetadata)) return;

		if ((fileMetadata.size || 0) > TEXT_PREVIEW_MAX_BYTES) {
			textPreviewError = tr('preview.textOnlyLimit', {
				limit: formatPreviewLimit(TEXT_PREVIEW_MAX_BYTES)
			});
			return;
		}

		isLoadingTextPreview = true;

		try {
			const { decrypted } = await downloadAndDecryptFile(fileId, key, token, async () => {});
			const previewText = (await decrypted.text()).replace(/\r\n/g, '\n');

			if (requestId !== previewRequestId) return;

			const clippedPreview = previewText.slice(0, TEXT_PREVIEW_MAX_CHARS);
			textPreview = clippedPreview;
			isTextPreviewTruncated = previewText.length > TEXT_PREVIEW_MAX_CHARS;
			const renderedPreview = renderTextPreview(clippedPreview, fileMetadata);
			textPreviewHtml = renderedPreview.html;
			textPreviewMode = renderedPreview.mode;
		} catch (error) {
			if (requestId !== previewRequestId) return;
			console.error('Text preview error:', error);
			const unavailableMessage = getUnavailableDownloadMessage(error);
			if (unavailableMessage) {
				metadata = { error: unavailableMessage };
				return;
			}
			textPreviewError = tr('preview.textLoadError');
		} finally {
			if (requestId === previewRequestId) {
				isLoadingTextPreview = false;
			}
		}
	}

	async function loadImagePreview(
		fileId: string,
		key: string,
		token: string,
		fileMetadata: FileMetadata,
		requestId: number
	) {
		if (!isImagePreviewable(fileMetadata)) return;

		if ((fileMetadata.size || 0) > IMAGE_PREVIEW_MAX_BYTES) {
			imagePreviewError = tr('preview.imageOnlyLimit', {
				limit: formatPreviewLimit(IMAGE_PREVIEW_MAX_BYTES)
			});
			return;
		}

		isLoadingImagePreview = true;

		try {
			const { decrypted } = await downloadAndDecryptFile(fileId, key, token, async () => {});
			const previewType = resolveImagePreviewType(fileMetadata);
			const previewBlob =
				previewType && decrypted.type !== previewType
					? new Blob([decrypted], { type: previewType })
					: decrypted;
			const objectUrl = URL.createObjectURL(previewBlob);

			if (requestId !== previewRequestId) {
				URL.revokeObjectURL(objectUrl);
				return;
			}

			imagePreviewUrl = objectUrl;
		} catch (error) {
			if (requestId !== previewRequestId) return;
			console.error('Image preview error:', error);
			const unavailableMessage = getUnavailableDownloadMessage(error);
			if (unavailableMessage) {
				metadata = { error: unavailableMessage };
				return;
			}
			imagePreviewError = tr('preview.imageLoadError');
		} finally {
			if (requestId === previewRequestId) {
				isLoadingImagePreview = false;
			}
		}
	}

	async function loadPreviews(
		fileId: string,
		key: string,
		token: string,
		fileMetadata: FileMetadata,
		requestId: number
	) {
		const previewTasks: Promise<void>[] = [];

		if (isTextPreviewable(fileMetadata)) {
			previewTasks.push(loadTextPreview(fileId, key, token, fileMetadata, requestId));
		}

		if (isImagePreviewable(fileMetadata)) {
			previewTasks.push(loadImagePreview(fileId, key, token, fileMetadata, requestId));
		}

		if (previewTasks.length > 0) {
			await Promise.all(previewTasks);
		}
	}

	async function getMetadata() {
		const requestId = resetPreviews();

		try {
			const fileId = getCurrentFileId();
			const activeKey = encryptionKey;
			const hmacToken = await generateHmacToken(fileId, activeKey);
			const metadataResponse = await fetchMetadata(fileId, activeKey, hmacToken);
			metadata = metadataResponse.metadata;
			fileSize = metadataResponse.size?.toString();
			void loadPreviews(fileId, activeKey, hmacToken, metadataResponse.metadata, requestId);
		} catch (error) {
			console.error('Metadata error:', error);
			encryptionKey = '';
			manualKeyInput = '';
			metadata = {
				error:
					tr('dl.metadataError')
			};
		} finally {
			isLoading = false;
		}
	}

	function setEncryptionKey(key: string) {
		encryptionKey = key;
		resetPreviews();
		if (browser) replaceState('', window.location.pathname);
	}

	async function handleManualKeySubmit() {
		if (!manualKeyInput.trim()) return;
		const key = validateAndExtractKey(manualKeyInput.trim());
		if (key) {
			setEncryptionKey(key);
			await getMetadata();
		} else {
			keyError = tr('key.invalid');
		}
	}

	async function initiateDownload() {
		if (!encryptionKey || isDownloading || !metadata || metadata.error) return;
		isDownloading = true;
		downloadError = null;

		try {
			const fileId = getCurrentFileId();
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
			const chunks: BlobPart[] = [];
			let receivedLength = 0;

			while (true) {
				const { done, value } = await reader.read();
				if (done) break;
				if (value) {
					chunks.push(new Uint8Array(value));
					receivedLength += value.length;
				}
			}

			if (receivedLength === 0) {
				throw new Error(tr('dl.decryptError'));
			}

			const blob = new Blob(chunks, {
				type: fileMetadata.contentType || 'application/octet-stream'
			});

			if (blob.size === 0) {
				throw new Error(tr('dl.decryptError'));
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
					tr('dl.deleteNetworkError');
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

			const fileId = getCurrentFileId();

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
						keyError = tr('key.invalid');
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

	onDestroy(() => {
		resetPreviews();
	});

	let canDownload = $derived(!!(
		metadata &&
		!metadata.error &&
		!isDownloading &&
		!isDownloadComplete &&
		encryptionKey
	));

	run(() => {
		void manualKeyInput;
		keyError = null;
	});
</script>

<div class="page-container">
	<div class="container">
		<div class="download-section">
			<h1><a href="/"><span>{$t('dl.titleLink')}</span></a>{$t('dl.titleAfter')}</h1>
			<p class="description">
				{$t('dl.description')}
			</p>

			{#if isLoading}
				<LoadingSpinner message="" />
			{/if}

			{#if downloadError}
				<ErrorMessage message={downloadError} />
			{:else if metadata?.error}
				<ErrorMessage message={metadata.error} />
			{:else if metadata?.filename}
				{#if isImagePreviewable(metadata)}
					<div class="preview-card" in:fly={{ y: 12, duration: 240 }}>
						<div class="preview-header">
							<h2>{$t('preview.title')}</h2>
							<span class="preview-badge">{$t('preview.image')}</span>
						</div>

						{#if isLoadingImagePreview}
							<div class="preview-loading">
								<LoadingSpinner message={$t('preview.loadingImage')} />
							</div>
						{:else if imagePreviewUrl}
							<img
								class="image-preview"
								src={imagePreviewUrl}
								alt={$t('preview.imageAlt', { filename: metadata.filename })}
								use:fitViewport={{ reserveSelector: '.file-row', bottomGap: 48, minHeight: 200 }}
							/>
						{:else if imagePreviewError}
							<p class="preview-note">{imagePreviewError}</p>
						{/if}
					</div>
				{/if}

				{#if isTextPreviewable(metadata)}
					<div class="preview-card" in:fly={{ y: 12, duration: 240 }}>
						<div class="preview-header">
							<h2>{$t('preview.title')}</h2>
							<span class="preview-badge">{$t('preview.text')}</span>
						</div>

						{#if isLoadingTextPreview}
							<div class="preview-loading">
								<LoadingSpinner message={$t('preview.loadingText')} />
							</div>
						{:else if textPreview !== null}
							{#if textPreview.length > 0}
								<div class="text-preview-wrap">
									<button
										type="button"
										class="copy-preview-btn"
										class:copied={copyState === 'copied'}
										class:error={copyState === 'error'}
										onclick={copyTextPreview}
										aria-label={$t('preview.copyAria')}
										title={$t('preview.copyAria')}
									>
										{#if copyState === 'copied'}
											{$t('common.copied')}
										{:else if copyState === 'error'}
											{$t('common.error')}
										{:else}
											<svg
												width="14"
												height="14"
												viewBox="0 0 24 24"
												fill="none"
												stroke="currentColor"
												stroke-width="2"
												stroke-linecap="round"
												stroke-linejoin="round"
												aria-hidden="true"
											>
												<rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
												<path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
											</svg>
											<span>{$t('common.copy')}</span>
										{/if}
									</button>
									{#if textPreviewMode === 'table'}
										<div
											class="table-preview"
											use:fitViewport={{
												reserveSelector: '.file-row',
												bottomGap: 48,
												minHeight: 200
											}}
										>
											{@html textPreviewHtml}
										</div>
									{:else}
										<pre
											class="text-preview syntax-preview"
											use:fitViewport={{
												reserveSelector: '.file-row',
												bottomGap: 48,
												minHeight: 200
											}}>{@html textPreviewHtml}</pre>
									{/if}
								</div>
							{:else}
								<p class="preview-note">{$t('preview.emptyTextFile')}</p>
							{/if}

							{#if isTextPreviewTruncated}
								<p class="preview-note">{$t('preview.truncated')}</p>
							{/if}
						{:else if textPreviewError}
							<p class="preview-note">{textPreviewError}</p>
						{/if}
					</div>
				{/if}

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
								></div>
							</div>
						{/if}
					</div>

					<!-- Right: download button → spinner → checkmark -->
					<div class="col-action">
						{#if isDownloadComplete}
							<div class="checkmark" title={$t('download.completeTitle')}>
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
							<div class="spinner" aria-label={$t('download.downloadingAria')}></div>
						{:else}
							<button class="download-btn" onclick={initiateDownload} disabled={!canDownload}>
								{$t('common.download')}
							</button>
						{/if}
					</div>
				</div>

				{#if isDownloadComplete}
					<p class="deleted-notice" in:fly={{ y: 6, duration: 250 }}>
						{$t('download.fileDeleted')}
					</p>
				{/if}

				{#if deletionError}
					<ErrorMessage message={deletionError} />
				{/if}
			{/if}

			{#if !encryptionKey && !isDownloadComplete && !isLoading}
				<form class="key-prompt" onsubmit={preventDefault(handleManualKeySubmit)}>
					<h3>{$t('key.requiredTitle')}</h3>
					<p class="hint">
						{$t('key.hint')}
					</p>
					<div class="input-group">
						<input
							type="text"
							class="key-input"
							placeholder={$t('key.placeholder')}
							bind:value={manualKeyInput}
							disabled={isDownloading}
						/>
						<button type="submit" class="button" disabled={!manualKeyInput.trim() || isDownloading}>
							{$t('key.decrypt')}
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
		padding: 1rem 2rem;
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
		margin-top: 0;
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
		/* margin-bottom: 1.5rem; */
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

	.preview-card {
		margin-top: 1rem;
		padding: 1rem 1.25rem;
		border: 1px solid #e5e7eb;
		border-radius: 10px;
		background: #fff;
	}

	.preview-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		margin-bottom: 0.875rem;
	}

	.preview-header h2 {
		margin: 0;
		font-size: 0.9375rem;
		font-weight: 600;
		color: #111827;
	}

	.preview-badge {
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: #166534;
		background: rgba(34, 197, 94, 0.12);
		border-radius: 999px;
		padding: 0.25rem 0.625rem;
	}

	.preview-loading {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 8rem;
	}

	.text-preview {
		margin: 0;
		padding: 1rem;
		border-radius: 8px;
		background: #f8fafc;
		border: 1px solid #e5e7eb;
		color: #111827;
		font-size: 0.875rem;
		line-height: 1.6;
		font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
		white-space: pre-wrap;
		word-break: break-word;
		overflow: auto;
	}

	.text-preview-wrap {
		position: relative;
	}

	.copy-preview-btn {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
		z-index: 2;
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.3rem 0.6rem;
		font-size: 0.75rem;
		font-weight: 600;
		color: #1f2937;
		background: rgba(255, 255, 255, 0.92);
		border: 1px solid #e5e7eb;
		border-radius: 6px;
		cursor: pointer;
		backdrop-filter: blur(2px);
		transition: background 120ms ease, color 120ms ease, border-color 120ms ease;
	}

	.copy-preview-btn:hover {
		background: #fff;
		border-color: #cbd5e1;
	}

	.copy-preview-btn:focus-visible {
		outline: 2px solid #2563eb;
		outline-offset: 2px;
	}

	.copy-preview-btn.copied {
		color: #166534;
		border-color: rgba(34, 197, 94, 0.5);
		background: rgba(34, 197, 94, 0.12);
	}

	.copy-preview-btn.error {
		color: #b91c1c;
		border-color: rgba(239, 68, 68, 0.5);
		background: rgba(239, 68, 68, 0.1);
	}

	:global(.syntax-preview .tok-key),
	:global(.syntax-preview .tok-attr) {
		color: #0f766e;
	}

	:global(.syntax-preview .tok-string) {
		color: #9a3412;
	}

	:global(.syntax-preview .tok-number) {
		color: #7c3aed;
	}

	:global(.syntax-preview .tok-bool),
	:global(.syntax-preview .tok-null) {
		color: #b45309;
	}

	:global(.syntax-preview .tok-comment) {
		color: #6b7280;
		font-style: italic;
	}

	:global(.syntax-preview .tok-keyword),
	:global(.syntax-preview .tok-tag) {
		color: #1d4ed8;
	}

	:global(.syntax-preview .tok-punct) {
		color: #475569;
	}

	.image-preview {
		display: block;
		width: 100%;
		object-fit: contain;
		border-radius: 8px;
		border: 1px solid #e5e7eb;
		background: #f8fafc;
	}

	.table-preview {
		overflow: auto;
		border: 1px solid #e5e7eb;
		border-radius: 8px;
		background: #f8fafc;
	}

	.table-preview :global(.preview-table) {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
		line-height: 1.45;
	}

	.table-preview :global(th),
	.table-preview :global(td) {
		padding: 0.625rem 0.75rem;
		border-bottom: 1px solid #e5e7eb;
		border-right: 1px solid #e5e7eb;
		text-align: left;
		vertical-align: top;
		white-space: nowrap;
	}

	.table-preview :global(th) {
		position: sticky;
		top: 0;
		background: #eef2ff;
		color: #1f2937;
		font-weight: 600;
	}

	.table-preview :global(tr:last-child td) {
		border-bottom: none;
	}

	.table-preview :global(th:last-child),
	.table-preview :global(td:last-child) {
		border-right: none;
	}

	.preview-note {
		margin: 0;
		font-size: 0.8125rem;
		line-height: 1.5;
		color: #6b7280;
	}

	.text-preview + .preview-note {
		margin-top: 0.75rem;
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

	@media (prefers-color-scheme: dark) {
		.progress-track {
			background: #374151;
		}
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

		.preview-card {
			padding: 0.875rem;
		}

		.preview-header {
			align-items: flex-start;
			flex-direction: column;
		}
	}
</style>
