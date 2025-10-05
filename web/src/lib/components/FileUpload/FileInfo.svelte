<script lang="ts">
	import { fade, fly } from 'svelte/transition';
	import { onMount, onDestroy } from 'svelte';

	export let fileName: string = '';
	export let fileSize: string = '';
	export let isVisible: boolean = true;
	export let onRemove: (() => void) | undefined = undefined;
	export let downloadProgress: number = 0;
	export let downloadMessage: string = '';
	export let isDownloading: boolean = false;
	export let uploadProgress: number = 0;
	export let uploadMessage: string = '';
	export let isUploading: boolean = false;

	// For smooth animation
	let displayProgress: number = 0;
	let animationFrame: number;

	// Helper function to format file size
	function formatFileSize(bytes: number): string {
		if (!bytes) return '';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		let size = bytes;
		let unitIndex = 0;

		while (size >= 1024 && unitIndex < units.length - 1) {
			size /= 1024;
			unitIndex++;
		}

		return `${size.toFixed(1)} ${units[unitIndex]}`;
	}

	// Get the current progress based on what operation is active
	$: currentProgress = isUploading ? uploadProgress : downloadProgress;
	$: currentMessage = isUploading ? uploadMessage : downloadMessage;
	$: isActive = isUploading || isDownloading;

	// Update display progress smoothly using animation frames
	function updateDisplayProgress() {
		// Calculate the difference between target and current
		const diff = currentProgress - displayProgress;

		// If we're very close to the target, just snap to it
		if (Math.abs(diff) < 0.2) {
			displayProgress = currentProgress;
		} else {
			// Otherwise move a percentage of the remaining distance
			displayProgress += diff * 0.1;
		}

		// Continue animation if we haven't reached the target
		if (displayProgress !== currentProgress) {
			animationFrame = requestAnimationFrame(updateDisplayProgress);
		}
	}

	// Watch for changes in progress and start animation
	$: if (currentProgress !== displayProgress) {
		// Cancel any existing animation
		if (animationFrame) {
			cancelAnimationFrame(animationFrame);
		}
		// Start new animation
		animationFrame = requestAnimationFrame(updateDisplayProgress);
	}

	onMount(() => {
		// Initialize display progress
		displayProgress = currentProgress;
	});

	onDestroy(() => {
		// Clean up any pending animation frames
		if (animationFrame) {
			cancelAnimationFrame(animationFrame);
		}
	});
</script>

{#if isVisible}
	<div class="file-info" in:fly={{ y: 20, duration: 300 }} out:fade={{ duration: 200 }}>
		<!-- Progress background overlay -->
		{#if isActive}
			<div class="progress-background" style="width: {displayProgress}%"></div>
		{/if}

		{#if onRemove}
			<button class="remove-button" on:click={onRemove} aria-label="Remove file">
				<svg
					width="20"
					height="20"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<circle cx="12" cy="12" r="10" />
					<line x1="15" y1="9" x2="9" y2="15" />
					<line x1="9" y1="9" x2="15" y2="15" />
				</svg>
			</button>
		{/if}
		<div class="file-details">
			<div class="file-icon">
				<svg
					width="24"
					height="24"
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
			<div class="file-info-content">
				<div class="filename">{fileName}</div>
				{#if fileSize}
					<div class="filesize">
						{typeof fileSize === 'number' ? formatFileSize(fileSize) : fileSize}
					</div>
				{/if}
				{#if isActive && currentMessage}
					<div class="progress-message">{currentMessage}</div>
				{/if}
				{#if isActive}
					<div class="progress-text">{Math.round(displayProgress)}%</div>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	.file-info {
		background-color: white;
		border-radius: 8px;
		padding: 1rem;
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
		position: relative;
		border: 1px solid #eee;
		transition: all 0.2s ease;
		margin-bottom: 2rem;
		overflow: hidden; /* Ensure progress background doesn't overflow */
	}

	.file-info:hover {
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
	}

	.progress-background {
		position: absolute;
		top: 0;
		left: 0;
		height: 100%;
		background: linear-gradient(
			90deg,
			rgba(64, 184, 123, 0.2) 0%,
			rgba(64, 184, 123, 0.3) 50%,
			rgba(64, 184, 123, 0.2) 100%
		);
		transition: width 0.3s linear(0.4, 0, 0.2, 1);
		background-size: 200% 100%;
		background-position: 0% 0%;
		animation: shimmer 2s infinite;
		z-index: 1;
	}

	@keyframes shimmer {
		to {
			background-position: 200% 0%;
		}
	}

	.remove-button {
		position: absolute;
		top: -10px;
		right: -10px;
		background: #000;
		border: none;
		border-radius: 50%;
		width: 24px;
		height: 24px;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		padding: 0;
		color: white;
		transition: transform 0.2s ease;
		z-index: 3; /* Above progress background */
	}

	.remove-button:hover {
		transform: scale(1.1);
	}

	.file-details {
		display: flex;
		align-items: center;
		gap: 1rem;
		position: relative;
		z-index: 2; /* Above progress background */
	}

	.file-icon {
		color: #666;
	}

	.file-info-content {
		flex: 1;
	}

	.filename {
		font-weight: 500;
		margin-bottom: 0.25rem;
		word-break: break-all;
	}

	.filesize {
		color: #666;
		font-size: 0.875rem;
		margin-bottom: 0.25rem;
	}

	.progress-message {
		color: var(--primary-green, #40b87b);
		font-size: 0.875rem;
		font-weight: 500;
		margin-bottom: 0.25rem;
	}

	.progress-text {
		color: var(--primary-green, #40b87b);
		font-size: 0.875rem;
		font-weight: 600;
	}
</style>
