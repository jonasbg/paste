<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	export let progress: number = 0;
	export let message: string = '';
	export let isVisible: boolean = false;

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
	<div class="progress-title">{message}</div>
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
	}

	.progress-title {
			font-size: 1.25rem;
			margin-bottom: 1rem;
			font-weight: 500;
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