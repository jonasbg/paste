<script lang="ts">
	import { preventDefault, createBubbler, stopPropagation } from 'svelte/legacy';

	const bubble = createBubbler();
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$lib/env';
	import { t, tr } from '$lib/i18n';
	import { FileProcessor } from '$lib/services/fileProcessor';
	import { uploadEncryptedFile } from '$lib/services/encryptionService';
	import { fetchMetadata } from '$lib/services/fileService';
	import { downloadSaveAndDelete } from '$lib/services/downloadService';
	import { generateHmacToken } from '$lib/utils/hmacUtils';
	import { generatePassphrase, DEFAULT_PASSPHRASE_WORDS } from '$lib/utils/wordlist';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import PassphraseShare from '$lib/components/PassphraseShare/PassphraseShare.svelte';
	import { fade, fly, slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import { configStore } from '$lib/stores/config';
	import {
		type FileMetadata,
		isImagePreviewable,
		isTextPreviewable
	} from '$lib/utils/filePreview';
	import { fitViewport } from '$lib/actions/fitViewport';
	import { FilePreviews } from '$lib/state/filePreviews.svelte';
	import { SmoothProgress } from '$lib/state/smoothProgress.svelte';

	/**
	 * Upload flow. Exactly one of these at a time; each state carries only the
	 * data valid in it (the file being uploaded, the error, the share result).
	 */
	type UploadState =
		| { kind: 'idle' }
		| { kind: 'selected'; file: File }
		| { kind: 'uploading'; file: File }
		| { kind: 'failed'; file: File; error: string }
		| { kind: 'shared'; file: File; passphrase: string; url: string };

	type ResolvedFile = {
		fileId: string;
		key: string;
		metadata: FileMetadata & { filename: string };
		fileSize: string;
	};

	/** Receive-by-passphrase flow, shown under the upload area. */
	type RetrieveState =
		/** Passphrase form. `error` is set when the last passphrase did not resolve. */
		| { kind: 'idle'; error: string | null }
		| { kind: 'resolving' }
		| { kind: 'ready'; file: ResolvedFile }
		| { kind: 'downloading'; file: ResolvedFile }
		| { kind: 'download-failed'; file: ResolvedFile; error: string }
		/** Saved to disk. `deleteError` is set when the server copy could not be removed. */
		| { kind: 'downloaded'; file: ResolvedFile; deleteError: string | null }
		/** The server no longer has the file (already downloaded, deleted or expired). */
		| { kind: 'unavailable'; message: string };

	let upload = $state<UploadState>({ kind: 'idle' });
	let retrieve = $state<RetrieveState>({ kind: 'idle', error: null });

	// Bumped whenever a flow is reset or restarted; async work started under an
	// older value drops its result instead of resurrecting a stale state.
	let uploadAttempt = 0;
	let retrieveAttempt = 0;

	// Upload progress data for the ProgressBar (not flow state).
	let uploadProgress = $state(0);
	let uploadMessage = $state('');
	// Validation message for a rejected file (too large / config missing).
	let fileSizeError = $state('');

	let passphraseInput = $state('');

	const previews = new FilePreviews();
	const dlProgress = new SmoothProgress();

	let fileInput: HTMLInputElement | undefined = $state();
	let dropZoneEl: HTMLDivElement | null = $state(null);
	let passphraseInputEl: HTMLInputElement | null = $state(null);

	let showPasteAffordance = $state(false);
	let pasteAffordanceState: 'idle' | 'reading' | 'denied' | 'empty' = $state('idle');
	let pasteAffordanceTimer: ReturnType<typeof setTimeout> | null = null;

	// Drag state
	let dragCounter = 0;
	let isDragging = $state(false);

	/** Upload is in progress or finished; the upload area must not change. */
	let uploadLocked = $derived(upload.kind === 'uploading' || upload.kind === 'shared');
	let uploadFile = $derived('file' in upload ? upload.file : null);
	let retrievedFile = $derived('file' in retrieve ? retrieve.file : null);
	/** A passphrase resolved to a file (or to "file is gone"); the page shows it instead of the drop zone. */
	let hasRetrievedFile = $derived(retrievedFile !== null || retrieve.kind === 'unavailable');
	/** Whether a newly picked/dropped/pasted file may become the upload selection. */
	let canAcceptFile = $derived(!uploadLocked && !hasRetrievedFile && retrieve.kind !== 'resolving');
	let retrieveDownloading = $derived(retrieve.kind === 'downloading');
	let retrieveDownloaded = $derived(retrieve.kind === 'downloaded');

	let maxFileSizeLabel = $derived($configStore.data?.max_file_size ?? '–');

	// Server-configured passphrase word count (PASSPHRASE_WORDS); generatePassphrase
	// clamps this to the safe [4,8] range, so the fallback only matters before config loads.
	let passphraseWordCount = $derived($configStore.data?.passphrase_words ?? DEFAULT_PASSPHRASE_WORDS);

	const fitPreview = {
		containerSelector: '.passphrase-panel',
		reserveSelector: '.file-row',
		bottomGap: 48,
		minHeight: 200
	};

	/** Validate a picked/dropped/pasted file and make it the upload selection. */
	function acceptFile(file: File): boolean {
		if (!canAcceptFile) return false;
		if (!$configStore.data) {
			fileSizeError = 'Unable to validate file size: configuration not loaded';
			return false;
		}
		const max = new FileProcessor().getMaxFileSize();
		if (file.size > max) {
			fileSizeError = tr('upload.fileTooLarge', { size: FileProcessor.formatFileSize(max) });
			return false;
		}
		upload = { kind: 'selected', file };
		fileSizeError = '';
		return true;
	}

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files?.length) {
			if (!acceptFile(input.files[0])) {
				input.value = '';
			}
		}
	}

	/** Back to an empty upload area (cancels the result of any in-flight upload). */
	function clearUpload() {
		uploadAttempt += 1;
		upload = { kind: 'idle' };
		uploadProgress = 0;
		uploadMessage = '';
		fileSizeError = '';
		if (fileInput) fileInput.value = '';
	}

	async function handleUpload() {
		if (upload.kind !== 'selected' && upload.kind !== 'failed') return;
		const file = upload.file;
		if (!$configStore.data) {
			fileSizeError = 'Unable to upload: configuration not loaded';
			return;
		}

		const max = new FileProcessor().getMaxFileSize();
		if (file.size > max) {
			fileSizeError = tr('upload.fileTooLarge', { size: FileProcessor.formatFileSize(max) });
			return;
		}

		const attempt = ++uploadAttempt;
		const isCurrent = () => attempt === uploadAttempt;
		upload = { kind: 'uploading', file };
		uploadProgress = 0;
		uploadMessage = '';

		// A fresh passphrase per attempt: a retry after a failure gets a new fileId,
		// which avoids colliding with a partial upload on the server.
		const passphrase = generatePassphrase(passphraseWordCount);

		try {
			const { initWasm, getWasmInstance } = await import('$lib/utils/wasm-loader');
			await initWasm();

			const wasm = getWasmInstance();
			if (!wasm || !wasm.deriveFromPassphrase) throw new Error('WASM not initialized');

			const keySize = parseInt($configStore.data.key_size) || 128;
			const derived = wasm.deriveFromPassphrase(passphrase, keySize);
			if (derived instanceof Error) throw derived;

			const { fileId, key } = derived as { fileId: string; key: string };

			await uploadEncryptedFile(
				file,
				key,
				async (progress, message) => {
					if (!isCurrent()) return;
					uploadProgress = progress;
					uploadMessage = message;
				},
				fileId
			);

			if (!isCurrent()) return;
			upload = {
				kind: 'shared',
				file,
				passphrase,
				url: `${window.location.origin}/${fileId}#key=${key}`
			};
			if (fileInput) fileInput.value = '';
		} catch (error) {
			if (!isCurrent()) return;
			upload = {
				kind: 'failed',
				file,
				error: error instanceof Error ? error.message : String(error)
			};
			uploadProgress = 0;
			uploadMessage = '';
		}
	}

	/** Resolve the passphrase in the form to a file and show it, ready to download. */
	async function handlePassphraseDownload() {
		const phrase = passphraseInput.trim();
		if (!phrase || retrieve.kind === 'resolving') return;
		// Never let a passphrase lookup replace an upload or download in flight.
		if (upload.kind === 'uploading' || retrieve.kind === 'downloading') return;

		const attempt = ++retrieveAttempt;
		retrieve = { kind: 'resolving' };
		previews.reset();
		dlProgress.reset();

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
			if (attempt !== retrieveAttempt) return;

			clearUpload();
			retrieve = {
				kind: 'ready',
				file: { fileId, key, metadata: response.metadata, fileSize: response.size ?? '' }
			};
			void previews.load({
				fileId,
				key,
				token: hmacToken,
				metadata: response.metadata,
				onUnavailable: (message) => {
					// Only meaningful while the user is still looking at this file.
					if (attempt === retrieveAttempt && retrieve.kind === 'ready') {
						retrieve = { kind: 'unavailable', message };
					}
				}
			});
		} catch (err) {
			if (attempt !== retrieveAttempt) return;
			console.error('Passphrase resolve error:', err);
			retrieve = { kind: 'idle', error: tr('passphrase.invalidCode') };
		}
	}

	async function initiatePassphraseDownload() {
		if (retrieve.kind !== 'ready' && retrieve.kind !== 'download-failed') return;
		const file = retrieve.file;
		const attempt = retrieveAttempt;
		retrieve = { kind: 'downloading', file };
		dlProgress.begin();

		try {
			const outcome = await downloadSaveAndDelete(file.fileId, file.key, (p) => {
				if (attempt === retrieveAttempt) dlProgress.set(p);
			});
			if (attempt !== retrieveAttempt) return;
			dlProgress.complete();
			retrieve = {
				kind: 'downloaded',
				file,
				deleteError: outcome.deleted ? null : outcome.message
			};
		} catch (error) {
			if (attempt !== retrieveAttempt) return;
			dlProgress.reset();
			retrieve = { kind: 'download-failed', file, error: (error as Error).message };
		}
	}

	function resetAll() {
		clearUpload();
		retrieveAttempt += 1;
		retrieve = { kind: 'idle', error: null };
		passphraseInput = '';
		previews.reset();
		dlProgress.reset();
		isDragging = false;
		dragCounter = 0;
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
			if (upload.kind === 'selected') {
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
		if (!canAcceptFile) return;
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
		if (!canAcceptFile) return;
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
						acceptFile(
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
							acceptFile(
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
			acceptFile(new File([out], `pasted-text-${ts}.txt`, { type: 'text/plain' }));
			dismissPasteAffordance();
		} catch {
			pasteAffordanceState = 'denied';
		}
	}

	function handlePageDrop(event: DragEvent) {
		event.preventDefault();
		const files = event.dataTransfer?.files;
		if (files?.length) {
			acceptFile(files[0]);
		}
	}

	async function tryPassphraseFromHash(): Promise<boolean> {
		if (!browser || !window.location.hash) return false;
		const hashParams = new URLSearchParams(window.location.hash.slice(1));
		const passphrase = hashParams.get('passphrase');
		if (!passphrase) return false;
		// Leave the hash in place while an upload/download is running so the user
		// can open it once that finishes, instead of clobbering the transfer.
		if (upload.kind === 'uploading' || retrieve.kind === 'downloading') return false;
		history.replaceState(null, '', window.location.pathname);
		passphraseInput = passphrase;
		await handlePassphraseDownload();
		return true;
	}

	function handleHashChange() {
		void tryPassphraseFromHash();
	}

	onMount(async () => {
		if (!browser) return;

		// Register every page listener up front. These used to be added only when
		// the page was NOT opened via a #passphrase= link, so after resetting from
		// such a link, paste / Enter-to-upload / click-outside never worked.
		document.addEventListener('dragover', preventBrowserFileDrop);
		document.addEventListener('drop', handlePageDrop);
		window.addEventListener('paste', handlePaste);
		window.addEventListener('keydown', handleGlobalEnter);
		document.addEventListener('pointerdown', handleDocumentPointerDown, true);

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

		// Re-run passphrase handling on every hash change, not just on mount: editing
		// the URL to /#passphrase=... while already on the page fires `hashchange`
		// without reloading, so the on-mount check alone would never see it.
		window.addEventListener('hashchange', handleHashChange);

		// Handle #passphrase=... in URL — resolve inline
		await tryPassphraseFromHash();
	});

	onDestroy(() => {
		if (browser) {
			window.removeEventListener('paste', handlePaste);
			window.removeEventListener('keydown', handleGlobalEnter);
			window.removeEventListener('hashchange', handleHashChange);
			document.removeEventListener('dragover', preventBrowserFileDrop);
			document.removeEventListener('drop', handlePageDrop);
			document.removeEventListener('pointerdown', handleDocumentPointerDown, true);
		}
		if (pasteAffordanceTimer) clearTimeout(pasteAffordanceTimer);
		previews.reset();
		dlProgress.destroy();
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
		const files = event.dataTransfer?.files;
		if (files?.length) {
			acceptFile(files[0]);
		}
	}

	function handleZoneClick() {
		if (canAcceptFile) fileInput?.click();
	}

	async function handlePaste(event: ClipboardEvent) {
		const target = event.target as HTMLElement;
		if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') return;
		if (!canAcceptFile) return;

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
				acceptFile(new File([blob], filename, { type: item.type }));
				return;
			}

			if (item.kind === 'file' && !item.type.startsWith('image/')) {
				event.preventDefault();
				const file = item.getAsFile();
				if (file) {
					acceptFile(file);
					return;
				}
			}

			if (item.type === 'text/plain') {
				event.preventDefault();
				item.getAsString(async (text) => {
					if (!text.trim()) return;
					const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
					const blob = new Blob([text], { type: 'text/plain' });
					acceptFile(
						new File([blob], `pasted-text-${ts}.txt`, { type: 'text/plain' })
					);
				});
				return;
			}
		}
	}

	function handleGlobalEnter(event: KeyboardEvent) {
		if (event.key !== 'Enter' || event.defaultPrevented) return;
		if (upload.kind !== 'selected') return;
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
</script>

<div class="page-container">
	<div class="container">
		<div class="upload-section">
			<h1>
				{$t('home.titleBefore')}<a href="/" onclick={preventDefault(resetAll)}
					>{$t('home.titleLink')}</a
				>{$t('home.titleAfter')}
			</h1>

			{#if upload.kind !== 'shared'}
				<p class="description">
					{$t('home.description')}
				</p>

				{#if fileSizeError}
					<ErrorMessage message={fileSizeError} />
				{/if}

				{#if upload.kind === 'failed'}
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
								<span class="retry-filename">{upload.file.name}</span>
								<span class="retry-errmsg">{upload.error}</span>
							</div>
						</div>
						<div class="retry-actions">
							<button class="btn-retry" onclick={handleUpload}>{$t('common.retry')}</button>
							<button
								class="btn-dismiss-retry"
								onclick={clearUpload}
								aria-label={$t('common.cancel')}
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
								>
									<line x1="18" y1="6" x2="6" y2="18" />
									<line x1="6" y1="6" x2="18" y2="18" />
								</svg>
							</button>
						</div>
					</div>
				{/if}

				<!-- Row 1: Drop zone — slides away once a passphrase file is resolved, on upload error, or when a file is selected -->
				{#if upload.kind === 'idle' && !hasRetrievedFile}
					<div
						class="drop-zone"
						class:dragging={isDragging}
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
						aria-label={$t('upload.dropzoneAria')}
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
											{$t('paste.reading')}
										{:else if pasteAffordanceState === 'denied'}
											{$t('paste.denied')}
										{:else if pasteAffordanceState === 'empty'}
											{$t('paste.empty')}
										{:else}
											{$t('paste.idle')}
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
							disabled={!canAcceptFile}
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
							{$t('upload.dropPrimary')}
						</p>
						<p class="drop-secondary">{$t('upload.maxFileSize', { size: maxFileSizeLabel })}</p>
					</div>
				{/if}
			{/if}

			<!-- Selected file preview: shown after file chosen, before upload starts -->
			{#if upload.kind === 'selected'}
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
						<div class="file-name">{upload.file.name}</div>
						<div class="file-pre-size">{FileProcessor.formatFileSize(upload.file.size)}</div>
					</div>
					<div class="selected-col-action">
						<button class="btn-upload-now" onclick={handleUpload}>{$t('upload.uploadNow')}</button>
						<button
							class="btn-remove-file"
							onclick={clearUpload}
							aria-label={$t('upload.removeFile')}
						>
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
				isVisible={uploadLocked}
				isComplete={upload.kind === 'shared'}
				progress={uploadProgress}
				message={uploadMessage}
				fileName={uploadFile?.name ?? ''}
				fileSize={uploadFile ? FileProcessor.formatFileSize(uploadFile.size) : ''}
				fileSizeBytes={uploadFile?.size ?? 0}
			/>

			<PassphraseShare
				passphrase={upload.kind === 'shared' ? upload.passphrase : ''}
				secureUrl={upload.kind === 'shared' ? upload.url : ''}
				isVisible={upload.kind === 'shared'}
			/>

			<!-- Passphrase download panel -->
			{#if !uploadLocked}
				<div class="passphrase-panel" transition:slide={{ duration: 300, easing: cubicOut }}>
					<!-- "eller" separator fades away once a file is resolved -->
					{#if !hasRetrievedFile && upload.kind === 'idle'}
						<div class="horizontal-separator" out:fade={{ duration: 200 }}>
							<span>{$t('common.or')}</span>
						</div>
					{/if}

					{#if !hasRetrievedFile && upload.kind === 'idle'}
						<div class="copy-section" out:slide={{ duration: 250, easing: cubicOut }}>
							<p class="hint">{$t('passphrase.hint')}</p>
							{#if retrieve.kind === 'idle' && retrieve.error}
								<p class="passphrase-error">{retrieve.error}</p>
							{/if}
							<form onsubmit={preventDefault(handlePassphraseDownload)}>
								<div class="input-group">
									<input
										type="text"
										class="url-field"
										placeholder={$t('passphrase.placeholder')}
										bind:value={passphraseInput}
										bind:this={passphraseInputEl}
										onkeydown={handlePassphraseKeydown}
										disabled={retrieve.kind === 'resolving'}
									/>
									<button
										type="submit"
										class="button"
										disabled={!passphraseInput.trim() || retrieve.kind === 'resolving'}
									>
										{retrieve.kind === 'resolving'
											? $t('common.loadingShort')
											: $t('passphrase.findFile')}
									</button>
								</div>
							</form>
						</div>
					{/if}

					{#if retrieve.kind === 'unavailable'}
						<ErrorMessage message={retrieve.message} />
					{:else if retrievedFile}
						{#if isImagePreviewable(retrievedFile.metadata)}
							<div class="preview-card" in:fly={{ y: 12, duration: 240 }}>
								<div class="preview-header">
									<h2>{$t('preview.title')}</h2>
									<span class="preview-badge">{$t('preview.image')}</span>
								</div>

								{#if previews.image.status === 'loading'}
									<div class="preview-loading">
										<LoadingSpinner message={$t('preview.loadingImage')} />
									</div>
								{:else if previews.image.status === 'ready'}
									<img
										class="image-preview"
										src={previews.image.url}
										alt={$t('preview.imageAlt', { filename: retrievedFile.metadata.filename })}
										use:fitViewport={fitPreview}
									/>
								{:else if previews.image.status === 'error'}
									<p class="preview-note">{previews.image.message}</p>
								{/if}
							</div>
						{/if}

						{#if isTextPreviewable(retrievedFile.metadata)}
							<div class="preview-card" in:fly={{ y: 12, duration: 240 }}>
								<div class="preview-header">
									<h2>{$t('preview.title')}</h2>
									<span class="preview-badge">{$t('preview.text')}</span>
								</div>

								{#if previews.text.status === 'loading'}
									<div class="preview-loading">
										<LoadingSpinner message={$t('preview.loadingText')} />
									</div>
								{:else if previews.text.status === 'ready'}
									{#if previews.text.text.length > 0}
										<div class="text-preview-wrap">
											<button
												type="button"
												class="copy-preview-btn"
												class:copied={previews.copyState === 'copied'}
												class:error={previews.copyState === 'error'}
												onclick={() => previews.copyText()}
												aria-label={$t('preview.copyAria')}
												title={$t('preview.copyAria')}
											>
												{#if previews.copyState === 'copied'}
													{$t('common.copied')}
												{:else if previews.copyState === 'error'}
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
											{#if previews.text.mode === 'table'}
												<div class="table-preview" use:fitViewport={fitPreview}>
													{@html previews.text.html}
												</div>
											{:else}
												<pre
													class="text-preview syntax-preview"
													use:fitViewport={fitPreview}>{@html previews.text.html}</pre>
											{/if}
										</div>
									{:else}
										<p class="preview-note">{$t('preview.emptyTextFile')}</p>
									{/if}

									{#if previews.text.truncated}
										<p class="preview-note">{$t('preview.truncated')}</p>
									{/if}
								{:else if previews.text.status === 'error'}
									<p class="preview-note">{previews.text.message}</p>
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
								<div class="file-name">{retrievedFile.metadata.filename}</div>
								<div class="file-meta">
									<div class="meta-left">
										{#if retrievedFile.fileSize}
											<span class="size">{retrievedFile.fileSize}</span>
										{/if}
										{#if dlProgress.eta && retrieveDownloading}
											<span class="dot">·</span>
											<span class="eta">{dlProgress.eta}</span>
										{/if}
									</div>
									{#if retrieveDownloading || retrieveDownloaded}
										<span class="pct">{Math.round(dlProgress.display)}%</span>
									{/if}
								</div>
								{#if retrieveDownloading || retrieveDownloaded}
									<div class="progress-track">
										<div
											class="progress-fill"
											class:complete={retrieveDownloaded}
											style="width: {dlProgress.display}%"
										></div>
									</div>
								{/if}
							</div>

							<!-- Right: download button → spinner → checkmark -->
							<div class="col-action">
								{#if retrieveDownloaded}
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
								{:else if retrieveDownloading}
									<div class="spinner" aria-label={$t('download.downloadingAria')}></div>
								{:else}
									<button class="btn-last-ned" onclick={initiatePassphraseDownload}>
										{$t('common.download')}
									</button>
								{/if}
							</div>
						</div>

						{#if retrieve.kind === 'downloaded'}
							{#if retrieve.deleteError}
								<div class="download-error" in:fly={{ y: 8, duration: 200 }}>
									<ErrorMessage message={retrieve.deleteError} />
								</div>
							{:else}
								<p class="deleted-notice" in:fly={{ y: 6, duration: 250 }}>
									{$t('download.fileDeleted')}
								</p>
							{/if}
						{/if}

						{#if retrieve.kind === 'download-failed'}
							<div class="download-error" in:fly={{ y: 8, duration: 200 }}>
								<ErrorMessage message={retrieve.error} />
							</div>
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
		right: calc(0.5rem + 25px);
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
