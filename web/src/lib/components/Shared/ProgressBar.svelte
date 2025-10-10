<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	export let progress: number = 0;
	export let message: string = '';
	export let isVisible: boolean = false;
	export let fileName: string = '';
	export let fileSize: string = '';

	// For smooth animation
	let displayProgress: number = 0;
	let animationFrame: number | null = null;
	let lastLoggedProgress: number = -1;
	let lastUpdateTime: number = 0;

	// Simulate continuous progress between real updates
	function updateDisplayProgress() {
		const now = Date.now();
		const timeSinceLastUpdate = now - lastUpdateTime;

		// Calculate the difference between target and current
		const diff = progress - displayProgress;

		// If we're very close to the target, just snap to it
		if (Math.abs(diff) < 0.1) {
			displayProgress = progress;
		} else {
			// Smooth catch-up to the real progress
			displayProgress += diff * 0.15;
		}

		// Add subtle continuous progress when waiting for updates (simulate work)
		// Only if we haven't received an update in a while and we're not at 100%
		if (timeSinceLastUpdate > 800 && displayProgress < progress - 0.5 && progress < 100) {
			// Very slow creep forward to show activity
			const creepAmount = 0.02; // Very small increment
			if (displayProgress < progress - 2) {
				displayProgress += creepAmount;
			}
		}

		// Only log when progress changes significantly (avoid console spam)
		if (Math.abs(displayProgress - lastLoggedProgress) >= 1) {
			lastLoggedProgress = displayProgress;
			console.log('Progress updated:', Math.round(displayProgress), '–', message);
		}

		// Continue animation if we're visible and haven't reached 100%
		if (isVisible && displayProgress < 99.9) {
			animationFrame = requestAnimationFrame(updateDisplayProgress);
		} else {
			animationFrame = null;
		}
	}

	// Start/restart animation when progress changes or visibility changes
	$: if (isVisible && progress >= 0) {
		// Update the timestamp whenever progress changes
		lastUpdateTime = Date.now();

		// Start animation loop if not already running
		if (animationFrame === null && displayProgress < 99.9) {
			animationFrame = requestAnimationFrame(updateDisplayProgress);
		}
	}

	// Stop animation when not visible
	$: if (!isVisible && animationFrame !== null) {
		cancelAnimationFrame(animationFrame);
		animationFrame = null;
	}

	onMount(() => {
		// Initialize display progress
		displayProgress = progress;
		lastUpdateTime = Date.now();
	});

	onDestroy(() => {
		// Clean up any pending animation frames
		if (animationFrame !== null) {
			cancelAnimationFrame(animationFrame);
			animationFrame = null;
		}
	});
</script>

<div class="progress-container" style="display: {isVisible ? 'block' : 'none'}">
	{#if fileName}
		<div class="file-metadata">
			<div class="file-info-left">
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
				<span class="file-name">{fileName}</span>
			</div>
			{#if fileSize}
				<span class="file-size">{fileSize}</span>
			{/if}
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
		justify-content: space-between;
		gap: 0.5rem;
		margin-bottom: 1rem;
		font-size: 0.875rem;
		color: #666;
	}

	.file-info-left {
		display: flex;
		align-items: center;
		gap: 0.5rem;
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
		font-size: 1.175rem;
	}

	.progress-bar {
		width: 100%;
		height: 12px;
		background-color: #e0e0e0;
		border-radius: 6px;
		overflow: hidden;
		margin-bottom: 0.5rem;
	}

	.progress {
		height: 100%;
		background-color: var(--primary-green);
		transition: width 0.4s cubic-bezier(0.4, 0, 0.2, 1);
		background-image: linear-gradient(
			90deg,
			rgba(255, 255, 255, 0) 0%,
			rgba(255, 255, 255, 0.15) 50%,
			rgba(255, 255, 255, 0) 100%
		);
		background-size: 200% 100%;
		background-position: 0% 0%;
		animation: shimmer 2s infinite;
		will-change: width;
	}

	@keyframes shimmer {
		to {
			background-position: 200% 0%;
		}
	}

	.progress-text {
		margin-top: 0.75rem;
		font-size: 1.15rem;
		font-weight: 600;
		color: #333;
		text-align: left;
	}
</style>
