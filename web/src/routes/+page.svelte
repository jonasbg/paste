<script lang="ts">
	import { run, preventDefault, createBubbler, stopPropagation } from 'svelte/legacy';

	const bubble = createBubbler();
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$app/environment';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { uploadEncryptedFile } from '$lib/services/encryptionService';
	import {
		downloadAndDecryptFile,
		fetchMetadata,
		streamDownloadAndDecrypt
	} from '$lib/services/fileService';
	import { generateHmacToken } from '$lib/utils/hmacUtils';
	import { generatePassphrase, DEFAULT_PASSPHRASE_WORDS } from '$lib/utils/wordlist';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import PassphraseShare from '$lib/components/PassphraseShare/PassphraseShare.svelte';
	import { fade, fly, slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import { configStore } from '$lib/stores/config';
	import { renderTextPreview } from '$lib/utils/textPreview';
	import { isTextBased } from '$lib/utils/mimeType';

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
	const IMAGE_MIME_TYPES = {
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

	let fileInput: HTMLInputElement | undefined = $state();
	let dropZoneEl: HTMLDivElement | null = $state(null);
	let passphraseInputEl: HTMLInputElement | null = $state(null);
	let selectedFile: File | null = $state(null);
	let isUploading = $state(false);
	let uploadProgress = $state(0);
	let uploadMessage = $state('');
	let sharePassphrase = $state('');
	let shareUrl = $state('');
	let fileSizeError = $state('');
	let uploadError = $state('');
	let generatedPassphrase = '';

	// Passphrase download form
	let passphraseInput = $state('');
	let passphraseError = $state('');
	let isDerivingPassphrase = $state(false);

	// Inline passphrase download state
	let passphraseFileId = '';
	let passphraseKey = '';
	let passphraseFileMetadata: any = $state(null);
	let passphraseFileSizeStr = $state('');
	let isPassphraseDownloading = $state(false);
	let passphraseDownloadProgress = $state(0);
	let passphraseDisplayProgress = $state(0);
	let passphraseDownloadComplete = $state(false);
	let passphraseDownloadError = $state('');
	let passphraseDownloadStartTime = $state(0);
	let passphraseEta = $state('');
	let passphraseAnimFrame: number | undefined = $state();
	let passphraseTextPreview: string | null = $state(null);
	let passphraseTextPreviewHtml = $state('');
	let passphraseTextPreviewMode: 'pre' | 'table' = $state('pre');
	let passphraseTextPreviewError = $state('');
	let isLoadingPassphraseTextPreview = $state(false);
	let isPassphraseTextPreviewTruncated = $state(false);
	let showPasteAffordance = $state(false);
	let pasteAffordanceState: 'idle' | 'reading' | 'denied' | 'empty' = $state('idle');
	let pasteAffordanceTimer: ReturnType<typeof setTimeout> | null = null;
	let passphraseCopyState: 'idle' | 'copied' | 'error' = $state('idle');
	let passphraseCopyResetTimer: ReturnType<typeof setTimeout> | null = null;
	let passphraseImagePreviewUrl: string | null = $state(null);
	let passphraseImagePreviewError = $state('');
	let isLoadingPassphraseImagePreview = $state(false);
	let passphrasePreviewRequestId = 0;

	// Drag state
	let dragCounter = 0;
	let isDragging = $state(false);

	let maxFileSizeLabel = $derived($configStore.data?.max_file_size ?? '–');

	// Server-configured passphrase word count (PASSPHRASE_WORDS); generatePassphrase
	// clamps this to the safe [4,8] range, so the fallback only matters before config loads.
	let passphraseWordCount = $derived($configStore.data?.passphrase_words ?? DEFAULT_PASSPHRASE_WORDS);

	function formatEta(seconds: number): string {
		if (!isFinite(seconds) || seconds <= 0 || seconds > 3600) return '';
		if (seconds < 60) return `${Math.ceil(seconds)}s igjen`;
		const mins = Math.floor(seconds / 60);
		const secs = Math.ceil(seconds % 60);
		return `${mins}m${secs > 0 ? ` ${secs}s` : ''} igjen`;
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

		const ext = getFileExtension(fileMetadata.filename);
		return (IMAGE_MIME_TYPES as Record<string, string>)[ext] || null;
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
						node.closest('.passphrase-panel')?.querySelectorAll(currentOptions.reserveSelector) ??
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

	function resetPassphraseTextPreview() {
		passphraseTextPreview = null;
		passphraseTextPreviewHtml = '';
		passphraseTextPreviewMode = 'pre';
		passphraseTextPreviewError = '';
		isLoadingPassphraseTextPreview = false;
		isPassphraseTextPreviewTruncated = false;
		if (passphraseCopyResetTimer) {
			clearTimeout(passphraseCopyResetTimer);
			passphraseCopyResetTimer = null;
		}
		passphraseCopyState = 'idle';
	}

	async function copyPassphrasePreview() {
		if (passphraseTextPreview === null) return;
		try {
			await navigator.clipboard.writeText(passphraseTextPreview);
			passphraseCopyState = 'copied';
		} catch {
			passphraseCopyState = 'error';
		}
		if (passphraseCopyResetTimer) clearTimeout(passphraseCopyResetTimer);
		passphraseCopyResetTimer = setTimeout(() => {
			passphraseCopyState = 'idle';
			passphraseCopyResetTimer = null;
		}, 1800);
	}

	function resetPassphraseImagePreview() {
		if (passphraseImagePreviewUrl && browser) {
			URL.revokeObjectURL(passphraseImagePreviewUrl);
		}

		passphraseImagePreviewUrl = null;
		passphraseImagePreviewError = '';
		isLoadingPassphraseImagePreview = false;
	}

	function resetPassphrasePreviews(): number {
		passphrasePreviewRequestId += 1;
		resetPassphraseTextPreview();
		resetPassphraseImagePreview();
		return passphrasePreviewRequestId;
	}

	async function loadPassphraseTextPreview(
		fileId: string,
		key: string,
		token: string,
		fileMetadata: FileMetadata,
		requestId: number
	) {
		if (!isTextPreviewable(fileMetadata)) return;

		if ((fileMetadata.size || 0) > TEXT_PREVIEW_MAX_BYTES) {
			passphraseTextPreviewError = `Forhåndsvisning er bare tilgjengelig for tekstfiler opptil ${formatPreviewLimit(TEXT_PREVIEW_MAX_BYTES)}.`;
			return;
		}

		isLoadingPassphraseTextPreview = true;

		try {
			const { decrypted } = await downloadAndDecryptFile(fileId, key, token, async () => {});
			const previewText = (await decrypted.text()).replace(/\r\n/g, '\n');

			if (requestId !== passphrasePreviewRequestId) return;

			const clippedPreview = previewText.slice(0, TEXT_PREVIEW_MAX_CHARS);
			passphraseTextPreview = clippedPreview;
			isPassphraseTextPreviewTruncated = previewText.length > TEXT_PREVIEW_MAX_CHARS;
			const renderedPreview = renderTextPreview(clippedPreview, fileMetadata);
			passphraseTextPreviewHtml = renderedPreview.html;
			passphraseTextPreviewMode = renderedPreview.mode;
		} catch (error) {
			if (requestId !== passphrasePreviewRequestId) return;
			console.error('Passphrase text preview error:', error);
			const unavailableMessage = getUnavailableDownloadMessage(error);
			if (unavailableMessage) {
				passphraseFileMetadata = { error: unavailableMessage };
				return;
			}
			passphraseTextPreviewError = 'Kunne ikke laste forhåndsvisning av tekstfilen.';
		} finally {
			if (requestId === passphrasePreviewRequestId) {
				isLoadingPassphraseTextPreview = false;
			}
		}
	}

	async function loadPassphraseImagePreview(
		fileId: string,
		key: string,
		token: string,
		fileMetadata: FileMetadata,
		requestId: number
	) {
		if (!isImagePreviewable(fileMetadata)) return;

		if ((fileMetadata.size || 0) > IMAGE_PREVIEW_MAX_BYTES) {
			passphraseImagePreviewError = `Forhåndsvisning er bare tilgjengelig for bildefiler opptil ${formatPreviewLimit(IMAGE_PREVIEW_MAX_BYTES)}.`;
			return;
		}

		isLoadingPassphraseImagePreview = true;

		try {
			const { decrypted } = await downloadAndDecryptFile(fileId, key, token, async () => {});
			const previewType = resolveImagePreviewType(fileMetadata);
			const previewBlob =
				previewType && decrypted.type !== previewType
					? new Blob([decrypted], { type: previewType })
					: decrypted;
			const objectUrl = URL.createObjectURL(previewBlob);

			if (requestId !== passphrasePreviewRequestId) {
				URL.revokeObjectURL(objectUrl);
				return;
			}

			passphraseImagePreviewUrl = objectUrl;
		} catch (error) {
			if (requestId !== passphrasePreviewRequestId) return;
			console.error('Passphrase image preview error:', error);
			const unavailableMessage = getUnavailableDownloadMessage(error);
			if (unavailableMessage) {
				passphraseFileMetadata = { error: unavailableMessage };
				return;
			}
			passphraseImagePreviewError = 'Kunne ikke laste forhåndsvisning av bildefilen.';
		} finally {
			if (requestId === passphrasePreviewRequestId) {
				isLoadingPassphraseImagePreview = false;
			}
		}
	}

	async function loadPassphrasePreviews(
		fileId: string,
		key: string,
		token: string,
		fileMetadata: FileMetadata,
		requestId: number
	) {
		const previewTasks: Promise<void>[] = [];

		if (isTextPreviewable(fileMetadata)) {
			previewTasks.push(loadPassphraseTextPreview(fileId, key, token, fileMetadata, requestId));
		}

		if (isImagePreviewable(fileMetadata)) {
			previewTasks.push(loadPassphraseImagePreview(fileId, key, token, fileMetadata, requestId));
		}

		if (previewTasks.length > 0) {
			await Promise.all(previewTasks);
		}
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

	run(() => {
		if (passphraseDownloadProgress !== passphraseDisplayProgress) {
			if (passphraseDownloadStartTime === 0 && passphraseDownloadProgress > 0) {
				passphraseDownloadStartTime = Date.now();
			}
			if (passphraseAnimFrame) cancelAnimationFrame(passphraseAnimFrame);
			passphraseAnimFrame = requestAnimationFrame(animatePassphraseProgress);
		}
	});

	run(() => {
		if (passphraseDownloadComplete) {
			passphraseDisplayProgress = 100;
			passphraseEta = '';
		}
	});

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

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files?.length) {
			if (!validateAndSetFile(input.files[0])) {
				input.value = '';
			}
		}
	}

	function cleanupMemoryReferences() {
		if (sharePassphrase && selectedFile) {
			if (fileInput) fileInput.value = '';
		}
	}

	function clearUploadDraft() {
		selectedFile = null;
		fileSizeError = '';
		uploadError = '';
		if (fileInput) fileInput.value = '';
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

			if (!generatedPassphrase) generatedPassphrase = generatePassphrase(passphraseWordCount);

			const wasm = getWasmInstance();
			if (!wasm || !wasm.deriveFromPassphrase) throw new Error('WASM not initialized');

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
			generatedPassphrase = generatePassphrase(passphraseWordCount); // fresh passphrase → new fileId avoids server-side collision
		} finally {
			isUploading = false;
		}
	}

	async function handlePassphraseDownload() {
		const phrase = passphraseInput.trim();
		if (!phrase) return;

		isDerivingPassphrase = true;
		passphraseError = '';
		const previewRequestId = resetPassphrasePreviews();

		try {
			const { initWasm, getWasmInstance } = await import('$lib/utils/wasm-loader');
			await initWasm();
			const wasm = getWasmInstance();
			if (!wasm || !wasm.deriveFromPassphrase) throw new Error('WASM not initialized');

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

			clearUploadDraft();
			passphraseFileId = fileId;
			passphraseKey = key;
			passphraseFileMetadata = response.metadata;
			passphraseFileSizeStr = response.size?.toString() ?? '';
			void loadPassphrasePreviews(fileId, key, hmacToken, response.metadata, previewRequestId);
		} catch (err) {
			passphraseError = 'Ugyldig delingskode eller filen finnes ikke. Prøv igjen.';
			resetPassphrasePreviews();
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
		generatedPassphrase = generatePassphrase(passphraseWordCount);
	}

	function resetAll() {
		selectedFile = null;
		isUploading = false;
		uploadProgress = 0;
		uploadMessage = '';
		sharePassphrase = '';
		shareUrl = '';
		fileSizeError = '';
		uploadError = '';
		generatedPassphrase = generatePassphrase(passphraseWordCount);
		passphraseInput = '';
		passphraseError = '';
		passphraseFileId = '';
		passphraseKey = '';
		passphraseFileMetadata = null;
		passphraseFileSizeStr = '';
		isPassphraseDownloading = false;
		passphraseDownloadProgress = 0;
		passphraseDisplayProgress = 0;
		passphraseDownloadComplete = false;
		passphraseDownloadError = '';
		passphraseDownloadStartTime = 0;
		passphraseEta = '';
		resetPassphrasePreviews();
		isDragging = false;
		dragCounter = 0;
		if (fileInput) fileInput.value = '';
		if (passphraseAnimFrame) cancelAnimationFrame(passphraseAnimFrame);
	}

	function dismissUploadError() {
		uploadError = '';
		selectedFile = null;
		generatedPassphrase = generatePassphrase(passphraseWordCount);
		if (fileInput) fileInput.value = '';
	}

	function preventBrowserFileDrop(event: DragEvent) {
		// Prevent browser from navigating to dropped file outside our drop zone
		event.preventDefault();
	}

	function handlePassphraseKeydown(event: KeyboardEvent) {
		if (event.key === 'Tab' && !event.shiftKey && dropZoneEl) {
			event.preventDefault();
			dropZoneEl.focus();
		}
	}

	function handleDropZoneKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			event.preventDefault();
			if (selectedFile && !isUploading) {
				handleUpload();
			} else {
				handleZoneClick();
			}
			return;
		}
		if (event.key === 'Escape' && showPasteAffordance) {
			event.preventDefault();
			dismissPasteAffordance();
			return;
		}
		if (event.key === 'Tab' && event.shiftKey && passphraseInputEl) {
			event.preventDefault();
			passphraseInputEl.focus();
		}
	}

	function handleDropZoneContextMenu(event: MouseEvent) {
		if (isUploading || sharePassphrase) return;
		event.preventDefault();
		event.stopPropagation();
		openPasteAffordance();
	}

	function openPasteAffordance() {
		pasteAffordanceState = 'idle';
		showPasteAffordance = true;
		if (pasteAffordanceTimer) clearTimeout(pasteAffordanceTimer);
		pasteAffordanceTimer = setTimeout(dismissPasteAffordance, 6000);
	}

	function dismissPasteAffordance() {
		showPasteAffordance = false;
		pasteAffordanceState = 'idle';
		if (pasteAffordanceTimer) {
			clearTimeout(pasteAffordanceTimer);
			pasteAffordanceTimer = null;
		}
	}

	function handleDocumentPointerDown(event: PointerEvent) {
		if (!showPasteAffordance) return;
		const target = event.target as HTMLElement | null;
		if (target?.closest('.paste-affordance')) return;
		dismissPasteAffordance();
	}

	async function pasteFromClipboard() {
		if (isUploading || sharePassphrase) return;
		if (!browser || !navigator.clipboard) {
			pasteAffordanceState = 'denied';
			return;
		}
		pasteAffordanceState = 'reading';
		const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);

		// Try rich clipboard items first (images, files)
		// @ts-ignore — read() not in all TS lib targets
		if (typeof navigator.clipboard.read === 'function') {
			try {
				// @ts-ignore
				const items = await navigator.clipboard.read();
				for (const item of items) {
					const imageType = item.types.find((t: string) => t.startsWith('image/'));
					if (imageType) {
						const blob = await item.getType(imageType);
						const ext = imageType.split('/')[1] || 'png';
						processClipboardFile(
							new File([blob], `screenshot-${ts}.${ext}`, { type: imageType })
						);
						dismissPasteAffordance();
						return;
					}
					if (item.types.includes('text/plain')) {
						const blob = await item.getType('text/plain');
						const text = await blob.text();
						if (text.trim()) {
							const out = new Blob([text], { type: 'text/plain' });
							processClipboardFile(
								new File([out], `pasted-text-${ts}.txt`, { type: 'text/plain' })
							);
							dismissPasteAffordance();
							return;
						}
					}
				}
			} catch {
				// fall through to readText
			}
		}

		try {
			const text = await navigator.clipboard.readText();
			if (!text.trim()) {
				pasteAffordanceState = 'empty';
				return;
			}
			const out = new Blob([text], { type: 'text/plain' });
			processClipboardFile(new File([out], `pasted-text-${ts}.txt`, { type: 'text/plain' }));
			dismissPasteAffordance();
		} catch {
			pasteAffordanceState = 'denied';
		}
	}

	function handlePageDrop(event: DragEvent) {
		event.preventDefault();
		if (isUploading || sharePassphrase) return;
		const files = event.dataTransfer?.files;
		if (files?.length) {
			validateAndSetFile(files[0]);
		}
	}

	onMount(async () => {
		if (!browser) return;

		// Block browser's default file-drop navigation on the whole page
		document.addEventListener('dragover', preventBrowserFileDrop);
		document.addEventListener('drop', handlePageDrop);

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

		generatedPassphrase = generatePassphrase(passphraseWordCount);
		window.addEventListener('paste', handlePaste);
		window.addEventListener('keydown', handleGlobalEnter);
		document.addEventListener('pointerdown', handleDocumentPointerDown, true);
	});

	onDestroy(() => {
		if (browser) {
			window.removeEventListener('paste', handlePaste);
			window.removeEventListener('keydown', handleGlobalEnter);
			document.removeEventListener('dragover', preventBrowserFileDrop);
			document.removeEventListener('drop', handlePageDrop);
			document.removeEventListener('pointerdown', handleDocumentPointerDown, true);
		}
		if (passphraseAnimFrame) cancelAnimationFrame(passphraseAnimFrame);
		if (pasteAffordanceTimer) clearTimeout(pasteAffordanceTimer);
		resetPassphrasePreviews();
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
			validateAndSetFile(files[0]);
		}
	}

	function handleZoneClick() {
		if (!isUploading && !sharePassphrase) fileInput?.click();
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

	function handleGlobalEnter(event: KeyboardEvent) {
		if (event.key !== 'Enter' || event.defaultPrevented) return;
		if (!selectedFile || isUploading || sharePassphrase) return;
		const target = event.target as HTMLElement | null;
		if (target) {
			const tag = target.tagName;
			if (
				tag === 'INPUT' ||
				tag === 'TEXTAREA' ||
				tag === 'BUTTON' ||
				tag === 'SELECT' ||
				tag === 'A' ||
				target.isContentEditable
			) {
				return;
			}
		}
		event.preventDefault();
		handleUpload();
	}

	function processClipboardFile(file: File) {
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
	}
</script>

<div class="page-container">
	<div class="container">
		<div class="upload-section">
			<h1>Vi <a href="/" onclick={preventDefault(resetAll)}>deler</a> filer sikkert</h1>

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
							<button class="btn-retry" onclick={handleUpload}>Prøv igjen</button>
							<button class="btn-dismiss-retry" onclick={dismissUploadError} aria-label="Avbryt">
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

				<!-- Row 1: Drop zone — slides away once a passphrase file is resolved, on upload error, or when a file is selected -->
				{#if !passphraseFileMetadata && !uploadError && !selectedFile}
					<div
						class="drop-zone"
						class:dragging={isDragging}
						class:uploading={isUploading}
						bind:this={dropZoneEl}
						onclick={handleZoneClick}
						oncontextmenu={handleDropZoneContextMenu}
						ondragenter={handleDragEnter}
						ondragleave={handleDragLeave}
						ondragover={handleDragOver}
						ondrop={handleDrop}
						role="button"
						tabindex="0"
						onkeydown={handleDropZoneKeydown}
						aria-label="Velg fil for opplasting"
						out:slide={{ duration: 350, easing: cubicOut }}
					>
						{#if showPasteAffordance}
							<div
								class="paste-affordance"
								role="menu"
								onclick={stopPropagation(bubble('click'))}
								oncontextmenu={stopPropagation(preventDefault(bubble('contextmenu')))}
								transition:fade={{ duration: 120 }}
							>
								<button
									type="button"
									class="paste-affordance-btn"
									onclick={stopPropagation(pasteFromClipboard)}
									disabled={pasteAffordanceState === 'reading'}
								>
									<svg
										width="16"
										height="16"
										viewBox="0 0 24 24"
										fill="none"
										stroke="currentColor"
										stroke-width="2"
										stroke-linecap="round"
										stroke-linejoin="round"
										aria-hidden="true"
									>
										<path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
										<rect x="8" y="2" width="8" height="4" rx="1" ry="1" />
									</svg>
									<span>
										{#if pasteAffordanceState === 'reading'}
											Leser utklippstavlen...
										{:else if pasteAffordanceState === 'denied'}
											Tilgang nektet
										{:else if pasteAffordanceState === 'empty'}
											Utklippstavlen er tom
										{:else}
											Lim inn fra utklippstavlen
										{/if}
									</span>
								</button>
							</div>
						{/if}
						<input
							type="file"
							bind:this={fileInput}
							onchange={handleFileSelect}
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

			<!-- Selected file preview: shown after file chosen, before upload starts -->
			{#if selectedFile && !isUploading && !sharePassphrase && !uploadError && !passphraseFileMetadata}
				<div class="selected-file-row" in:fly={{ y: 8, duration: 220 }}>
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
					<div class="col-info">
						<div class="file-name">{selectedFile.name}</div>
						<div class="file-pre-size">{FileProcessor.formatFileSize(selectedFile.size)}</div>
					</div>
					<div class="selected-col-action">
						<button class="btn-upload-now" onclick={handleUpload}>Last opp</button>
						<button class="btn-remove-file" onclick={removeFile} aria-label="Fjern fil">
							<svg
								width="15"
								height="15"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2.5"
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
					{#if !passphraseFileMetadata && !selectedFile}
						<div class="horizontal-separator" out:fade={{ duration: 200 }}>
							<span>eller</span>
						</div>
					{/if}

					{#if !passphraseFileMetadata && !selectedFile}
						<div class="copy-section" out:slide={{ duration: 250, easing: cubicOut }}>
							<p class="hint">Skriv inn delingskoden du har mottatt for å laste ned filen.</p>
							{#if passphraseError}
								<p class="passphrase-error">{passphraseError}</p>
							{/if}
							<form onsubmit={preventDefault(handlePassphraseDownload)}>
								<div class="input-group">
									<input
										type="text"
										class="url-field"
										placeholder="Skriv inn delingskoden din"
										bind:value={passphraseInput}
										bind:this={passphraseInputEl}
										onkeydown={handlePassphraseKeydown}
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
							{#if isImagePreviewable(passphraseFileMetadata)}
								<div class="preview-card" in:fly={{ y: 12, duration: 240 }}>
									<div class="preview-header">
										<h2>Forhåndsvisning</h2>
										<span class="preview-badge">Bilde</span>
									</div>

									{#if isLoadingPassphraseImagePreview}
										<div class="preview-loading">
											<LoadingSpinner message="Laster bildeforhåndsvisning..." />
										</div>
									{:else if passphraseImagePreviewUrl}
										<img
											class="image-preview"
											src={passphraseImagePreviewUrl}
											alt={`Forhåndsvisning av ${passphraseFileMetadata.filename}`}
											use:fitViewport={{
												reserveSelector: '.file-row',
												bottomGap: 48,
												minHeight: 200
											}}
										/>
									{:else if passphraseImagePreviewError}
										<p class="preview-note">{passphraseImagePreviewError}</p>
									{/if}
								</div>
							{/if}

							{#if isTextPreviewable(passphraseFileMetadata)}
								<div class="preview-card" in:fly={{ y: 12, duration: 240 }}>
									<div class="preview-header">
										<h2>Forhåndsvisning</h2>
										<span class="preview-badge">Tekst</span>
									</div>

									{#if isLoadingPassphraseTextPreview}
										<div class="preview-loading">
											<LoadingSpinner message="Laster tekstforhåndsvisning..." />
										</div>
									{:else if passphraseTextPreview !== null}
										{#if passphraseTextPreview.length > 0}
											<div class="text-preview-wrap">
												<button
													type="button"
													class="copy-preview-btn"
													class:copied={passphraseCopyState === 'copied'}
													class:error={passphraseCopyState === 'error'}
													onclick={copyPassphrasePreview}
													aria-label="Kopier tekst"
													title="Kopier tekst"
												>
													{#if passphraseCopyState === 'copied'}
														Kopiert
													{:else if passphraseCopyState === 'error'}
														Feil
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
														<span>Kopier</span>
													{/if}
												</button>
												{#if passphraseTextPreviewMode === 'table'}
													<div
														class="table-preview"
														use:fitViewport={{
															reserveSelector: '.file-row',
															bottomGap: 48,
															minHeight: 200
														}}
													>
														{@html passphraseTextPreviewHtml}
													</div>
												{:else}
													<pre
														class="text-preview syntax-preview"
														use:fitViewport={{
															reserveSelector: '.file-row',
															bottomGap: 48,
															minHeight: 200
														}}>{@html passphraseTextPreviewHtml}</pre>
												{/if}
											</div>
										{:else}
											<p class="preview-note">Denne tekstfilen er tom.</p>
										{/if}

										{#if isPassphraseTextPreviewTruncated}
											<p class="preview-note">
												Forhåndsvisningen er avkortet. Last ned filen for å se hele innholdet.
											</p>
										{/if}
									{:else if passphraseTextPreviewError}
										<p class="preview-note">{passphraseTextPreviewError}</p>
									{/if}
								</div>
							{/if}

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
											></div>
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
										<button class="btn-last-ned" onclick={initiatePassphraseDownload}>
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
		padding: 1rem 2rem;
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
		margin-top: 0;
		margin-bottom: 0.75rem;
	}

	h1 a {
		color: var(--primary-green);
		text-decoration: none;
	}

	.description {
		font-size: 1rem;
		line-height: 1.6;
		color: #555;
		/* margin-bottom: 1.5rem; */
	}

	.file-input {
		display: none;
	}

	/* ── Drop zone ── */
	.drop-zone {
		position: relative;
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

	.paste-affordance {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(243, 244, 246, 0.92);
		-webkit-backdrop-filter: blur(2px);
		backdrop-filter: blur(2px);
		border-radius: inherit;
		z-index: 5;
	}

	.paste-affordance-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.6rem 1rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: #fff;
		background: #111827;
		border: none;
		border-radius: 999px;
		cursor: pointer;
		box-shadow: 0 6px 18px rgba(0, 0, 0, 0.18);
		transition:
			transform 120ms ease,
			background 120ms ease,
			opacity 120ms ease;
	}

	.paste-affordance-btn:hover {
		background: #1f2937;
	}

	.paste-affordance-btn:active {
		transform: scale(0.98);
	}

	.paste-affordance-btn:disabled {
		opacity: 0.7;
		cursor: progress;
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
		/* margin-top: 1.5rem; */
		overflow: hidden;
	}

	.horizontal-separator {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.25rem;
		margin-top: 1.25rem;
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
		padding: 1.5rem 1rem;
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

	@media (prefers-color-scheme: dark) {
		.progress-track {
			background: rgb(209, 209, 209);
		}
	}

	.progress-fill {
		height: 100%;
		background: var(--primary-green);
		border-radius: 99px;
		transition: width 0.15s ease;
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

	/* ── Selected file preview (before upload) ── */
	.selected-file-row {
		display: grid;
		grid-template-columns: 52px 1fr auto;
		align-items: center;
		gap: 1rem;
		padding: 1rem 1.25rem;
		margin-top: 0.75rem;
	}

	.file-pre-size {
		font-size: 0.8125rem;
		color: #6b7280;
	}

	.selected-col-action {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.btn-upload-now {
		background-color: var(--primary-green);
		color: white;
		border: none;
		border-radius: 8px;
		padding: 0.5rem 1.125rem;
		font-size: 0.9375rem;
		font-weight: 600;
		font-family: inherit;
		cursor: pointer;
		white-space: nowrap;
		transition: all 0.2s ease;
	}

	.btn-upload-now:hover {
		transform: translateY(-1px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.12);
	}

	.btn-remove-file {
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

	.btn-remove-file:hover {
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

		.preview-card {
			padding: 0.875rem;
		}

		.preview-header {
			align-items: flex-start;
			flex-direction: column;
		}
	}
</style>
