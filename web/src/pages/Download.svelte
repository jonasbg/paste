<script lang="ts">
	import { preventDefault } from 'svelte/legacy';

	import { onDestroy, onMount } from 'svelte';
	import { browser } from '$lib/env';
	import { t, tr } from '$lib/i18n';
	import { initWasm } from '$lib/utils/wasm-loader';
	import { fetchMetadata } from '$lib/services/fileService';
	import { downloadSaveAndDelete } from '$lib/services/downloadService';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import { generateHmacToken } from '$lib/utils/hmacUtils';
	import {
		type FileMetadata,
		isImagePreviewable,
		isTextPreviewable
	} from '$lib/utils/filePreview';
	import { fitViewport } from '$lib/actions/fitViewport';
	import { FilePreviews } from '$lib/state/filePreviews.svelte';
	import { SmoothProgress } from '$lib/state/smoothProgress.svelte';
	import { fly } from 'svelte/transition';

	let { fileId }: { fileId: string } = $props();

	type ResolvedFile = { metadata: FileMetadata & { filename: string }; fileSize: string };

	/**
	 * The page is always in exactly one of these phases. Each phase carries only
	 * the data that is valid in it, so impossible combinations (e.g. "downloaded
	 * but still asking for a key") cannot be represented.
	 */
	type Phase =
		| { kind: 'initializing' }
		/** No usable key yet. `error` is set when a previous key failed to resolve. */
		| { kind: 'needs-key'; error: string | null }
		| { kind: 'loading-metadata' }
		| { kind: 'ready'; key: string; file: ResolvedFile }
		| { kind: 'downloading'; key: string; file: ResolvedFile }
		/** Saved to disk. `deleteError` is set when the server copy could not be removed. */
		| { kind: 'downloaded'; file: ResolvedFile; deleteError: string | null }
		/** The server no longer has the file (already downloaded, deleted or expired). */
		| { kind: 'unavailable'; message: string }
		| { kind: 'failed'; message: string };

	let phase = $state<Phase>({ kind: 'initializing' });
	let manualKeyInput = $state('');
	let keyError: string | null = $state(null);

	const previews = new FilePreviews();
	const progress = new SmoothProgress();

	let file = $derived('file' in phase ? phase.file : null);
	let isDownloading = $derived(phase.kind === 'downloading');
	let isDownloaded = $derived(phase.kind === 'downloaded');

	const fitPreview = {
		containerSelector: '.download-section',
		reserveSelector: '.file-row',
		bottomGap: 48,
		minHeight: 200
	};

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

	/** Resolve metadata for `key` and move to `ready`, or back to `needs-key` on failure. */
	async function resolveKey(key: string) {
		previews.reset();
		if (browser) window.history.replaceState(null, '', window.location.pathname);
		phase = { kind: 'loading-metadata' };

		try {
			if (!fileId) throw new Error('Missing file ID');
			const hmacToken = await generateHmacToken(fileId, key);
			const response = await fetchMetadata(fileId, key, hmacToken);
			phase = {
				kind: 'ready',
				key,
				file: { metadata: response.metadata, fileSize: response.size ?? '' }
			};
			void previews.load({
				fileId,
				key,
				token: hmacToken,
				metadata: response.metadata,
				onUnavailable: (message) => {
					// Only meaningful while the user is still looking at this file.
					if (phase.kind === 'ready' && phase.key === key) {
						phase = { kind: 'unavailable', message };
					}
				}
			});
		} catch (error) {
			console.error('Metadata error:', error);
			manualKeyInput = '';
			phase = { kind: 'needs-key', error: tr('dl.metadataError') };
		}
	}

	async function handleManualKeySubmit() {
		if (phase.kind !== 'needs-key' || !manualKeyInput.trim()) return;
		const key = validateAndExtractKey(manualKeyInput.trim());
		if (key) {
			await resolveKey(key);
		} else {
			keyError = tr('key.invalid');
		}
	}

	async function initiateDownload() {
		if (phase.kind !== 'ready') return;
		const { key, file } = phase;
		phase = { kind: 'downloading', key, file };
		progress.begin();

		try {
			const outcome = await downloadSaveAndDelete(fileId, key, (p) => progress.set(p));
			progress.complete();
			phase = {
				kind: 'downloaded',
				file,
				deleteError: outcome.deleted ? null : outcome.message
			};
			manualKeyInput = '';
			if (browser) window.history.replaceState({}, '', '/');
		} catch (error) {
			console.error('Download error:', error);
			progress.reset();
			phase = { kind: 'failed', message: (error as Error).message };
		}
	}

	onMount(async () => {
		if (!browser) return;

		try {
			await initWasm();
		} catch (error) {
			console.error('Failed to initialize:', error);
			phase = { kind: 'failed', message: 'Failed to initialize the application' };
			return;
		}

		let storedKey: string | null = null;
		try {
			storedKey = sessionStorage.getItem('paste_key_' + fileId);
			if (storedKey) sessionStorage.removeItem('paste_key_' + fileId);
		} catch {
			// Storage can be unavailable (private mode, blocked site data).
		}
		if (storedKey) {
			await resolveKey(storedKey);
			return;
		}

		const hashKey = window.location.hash
			? new URLSearchParams(window.location.hash.slice(1)).get('key')
			: null;
		if (hashKey) {
			const validatedKey = validateAndExtractKey(hashKey);
			if (validatedKey) {
				await resolveKey(validatedKey);
				return;
			}
			keyError = tr('key.invalid');
		}

		phase = { kind: 'needs-key', error: null };
	});

	onDestroy(() => {
		previews.reset();
		progress.destroy();
	});
</script>

<div class="page-container">
	<div class="container">
		<div class="download-section">
			<h1><a href="/"><span>{$t('dl.titleLink')}</span></a>{$t('dl.titleAfter')}</h1>
			<p class="description">
				{$t('dl.description')}
			</p>

			{#if phase.kind === 'initializing' || phase.kind === 'loading-metadata'}
				<LoadingSpinner message="" />
			{/if}

			{#if phase.kind === 'failed' || phase.kind === 'unavailable'}
				<ErrorMessage message={phase.message} />
			{:else if file}
				{#if isImagePreviewable(file.metadata)}
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
								alt={$t('preview.imageAlt', { filename: file.metadata.filename })}
								use:fitViewport={fitPreview}
							/>
						{:else if previews.image.status === 'error'}
							<p class="preview-note">{previews.image.message}</p>
						{/if}
					</div>
				{/if}

				{#if isTextPreviewable(file.metadata)}
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
						<div class="file-name">{file.metadata.filename}</div>
						<div class="file-meta">
							<div class="meta-left">
								{#if file.fileSize}
									<span class="size">{file.fileSize}</span>
								{/if}
								{#if progress.eta && isDownloading}
									<span class="dot">·</span>
									<span class="eta">{progress.eta}</span>
								{/if}
							</div>
							{#if isDownloading || isDownloaded}
								<span class="pct">{Math.round(progress.display)}%</span>
							{/if}
						</div>
						{#if isDownloading || isDownloaded}
							<div class="progress-track">
								<div
									class="progress-fill"
									class:complete={isDownloaded}
									style="width: {progress.display}%"
								></div>
							</div>
						{/if}
					</div>

					<!-- Right: download button → spinner → checkmark -->
					<div class="col-action">
						{#if isDownloaded}
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
							<button
								class="download-btn"
								onclick={initiateDownload}
								disabled={phase.kind !== 'ready'}
							>
								{$t('common.download')}
							</button>
						{/if}
					</div>
				</div>

				{#if phase.kind === 'downloaded'}
					{#if phase.deleteError}
						<ErrorMessage message={phase.deleteError} />
					{:else}
						<p class="deleted-notice" in:fly={{ y: 6, duration: 250 }}>
							{$t('download.fileDeleted')}
						</p>
					{/if}
				{/if}
			{/if}

			{#if phase.kind === 'needs-key'}
				{#if phase.error}
					<ErrorMessage message={phase.error} />
				{/if}
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
							oninput={() => (keyError = null)}
						/>
						<button type="submit" class="button" disabled={!manualKeyInput.trim()}>
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
