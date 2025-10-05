<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	export let progress: number = 0;
	export let message: string = '';
	export let isVisible: boolean = false;
	export let fileName: string = '';
	export let fileSize: string = '';

	// For smooth animation
	let displayProgress: number = 0;
	let animationFrame: number;
	let lastLoggedProgress: number = -1;

	// Update display progress smoothly using animation frames
	function updateDisplayProgress() {
		// Calculate the difference between target and current
		const diff = progress - displayProgress;

		// If we're very close to the target, just snap to it
		if (Math.abs(diff) < 0.2) {
			displayProgress = progress;
		} else {
			// Otherwise move a percentage of the remaining distance
			// Smaller value = smoother but slower animation
			displayProgress += diff * 0.1;
		}

		// Only log when progress changes significantly (avoid console spam)
		if (Math.abs(displayProgress - lastLoggedProgress) >= 1) {
			lastLoggedProgress = displayProgress;
			console.log('Progress updated:', Math.round(displayProgress), '–', message);
		}

		// Continue animation if we haven't reached the target
		if (displayProgress !== progress) {
			animationFrame = requestAnimationFrame(updateDisplayProgress);
		}
	}

	// Watch for changes in progress and start animation
	$: if (progress !== displayProgress) {
		// Cancel any existing animation
		if (animationFrame) {
			cancelAnimationFrame(animationFrame);
		}
		// Start new animation
		animationFrame = requestAnimationFrame(updateDisplayProgress);
	}

	onMount(() => {
		// Initialize display progress
		displayProgress = progress;
	});

	onDestroy(() => {
		// Clean up any pending animation frames
		if (animationFrame) {
			cancelAnimationFrame(animationFrame);
		}
	});
</script>

<div class="progress-container" style="display: {isVisible ? 'block' : 'none'}">
	{#if fileName}
		<div class="file-metadata">
			<div class="file-icon">
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
					<path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z" />
					<polyline points="13 2 13 9 20 9" />
				</svg>
			</div>
			<div class="file-details">
				<span class="file-name">{fileName}</span>
				{#if fileSize}
					<span class="file-size">({fileSize})</span>
				{/if}
			</div>
		</div>
	{/if}
	<!-- <div class="progress-title">{message}</div> Show message later if you want -->
	<div class="progress-bar">
		<div class="progress" style="width: {displayProgress}%"></div>
	</div>
	<div class="progress-text">{Math.round(displayProgress)}%</div>
</div>

<style>
	.progress-container {
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 2rem;
		border: 1px solid #e0e0e0;
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
	}

	.file-metadata {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 1rem;
		font-size: 0.875rem;
		color: #666;
	}

	.file-icon {
		color: #888;
	}

	.file-details {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}

	.file-name {
		font-weight: 600;
		color: #333;
		font-size: 1.125rem;
	}

	.file-size {
		color: #666;
	}

	.progress-bar {
		width: 100%;
		height: 8px;
		background-color: #e0e0e0;
		border-radius: 4px;
		overflow: hidden;
	}

	.progress {
		height: 100%;
		background-color: var(--primary-green);
		transition: width 0.3s linear(0.4, 0, 0.2, 1);
		background-image: linear-gradient(
			90deg,
			rgba(255, 255, 255, 0) 0%,
			rgba(255, 255, 255, 0.15) 50%,
			rgba(255, 255, 255, 0) 100%
		);
		background-size: 200% 100%;
		background-position: 0% 0%;
		animation: shimmer 2s infinite;
	}

	@keyframes shimmer {
		to {
			background-position: 200% 0%;
		}
	}

	.progress-text {
		margin-top: 0.5rem;
		font-size: 0.875rem;
		color: #666;
	}
</style>
