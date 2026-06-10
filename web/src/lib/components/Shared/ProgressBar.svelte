<script lang="ts">
	import { run } from 'svelte/legacy';

	import { onMount, onDestroy } from 'svelte';
	import { fly } from 'svelte/transition';
	import { tr } from '$lib/i18n';

	interface Props {
		progress?: number;
		message?: string;
		isVisible?: boolean;
		isComplete?: boolean;
		fileName?: string;
		fileSize?: string;
		fileSizeBytes?: number;
		onCancel?: (() => void) | undefined;
	}

	let {
		progress = 0,
		message = '',
		isVisible = false,
		isComplete = false,
		fileName = '',
		fileSize = '',
		fileSizeBytes = 0,
		onCancel = undefined
	}: Props = $props();

	let displayProgress: number = $state(0);
	let animationFrame: number | undefined = $state();
	let uploadStartTime: number = $state(0);
	let eta: string = $state('');

	function formatEta(seconds: number): string {
		if (!isFinite(seconds) || seconds <= 0 || seconds > 3600) return '';
		if (seconds < 60) return `${Math.ceil(seconds)}s ${tr('common.remaining')}`;
		const mins = Math.floor(seconds / 60);
		const secs = Math.ceil(seconds % 60);
		return `${mins}m${secs > 0 ? ` ${secs}s` : ''} ${tr('common.remaining')}`;
	}

	function updateDisplayProgress() {
		const diff = progress - displayProgress;
		if (Math.abs(diff) < 0.2) {
			displayProgress = progress;
		} else {
			displayProgress += diff * 0.1;
		}

		if (displayProgress > 2 && displayProgress < 99 && uploadStartTime > 0) {
			const elapsed = (Date.now() - uploadStartTime) / 1000;
			if (elapsed > 0.5) {
				const rate = displayProgress / elapsed;
				eta = formatEta((100 - displayProgress) / rate);
			}
		} else if (displayProgress >= 99) {
			eta = '';
		}

		if (displayProgress !== progress || (isVisible && !isComplete)) {
			animationFrame = requestAnimationFrame(updateDisplayProgress);
		}
	}

	run(() => {
		if (progress !== displayProgress) {
			if (uploadStartTime === 0 && progress > 0) {
				uploadStartTime = Date.now();
			}
			if (animationFrame) cancelAnimationFrame(animationFrame);
			animationFrame = requestAnimationFrame(updateDisplayProgress);
		}
	});

	run(() => {
		if (isComplete) {
			displayProgress = 100;
			eta = '';
		}
	});

	onMount(() => {
		displayProgress = progress;
	});

	onDestroy(() => {
		if (animationFrame) cancelAnimationFrame(animationFrame);
	});
</script>

{#if isVisible}
	<div class="upload-row" in:fly={{ y: 16, duration: 280 }}>
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

		<!-- Middle: name · meta · progress bar -->
		<div class="col-info">
			<div class="file-name">{fileName}</div>
			<div class="file-meta">
				<div class="meta-left">
					<span class="size">{fileSize}</span>
					{#if eta && !isComplete}
						<span class="dot">·</span>
						<span class="eta">{eta}</span>
					{/if}
				</div>
				<span class="pct">{Math.round(displayProgress)}%</span>
			</div>
			<div class="progress-track">
				<div class="progress-fill" class:complete={isComplete} style="width: {displayProgress}%"></div>
			</div>
		</div>

		<!-- Right: spinner while uploading, checkmark when done -->
		<div class="col-action">
			{#if isComplete}
				<div class="checkmark">
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
			{:else}
				<div class="spinner" aria-label="Laster opp..."></div>
			{/if}
		</div>
	</div>
{/if}

<style>
	.upload-row {
		display: grid;
		grid-template-columns: 52px 1fr 44px;
		align-items: center;
		gap: 1rem;
		background: #fff;
		/* border: 1px solid #e5e7eb; */
		border-radius: 10px;
		padding: 1rem 1.25rem;
		/* box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05); */
		margin-top: 1rem;
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
</style>
