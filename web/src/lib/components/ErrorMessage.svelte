<script lang="ts">
	export let message: string;
</script>

<div class="error-message">
	<svg
		xmlns="http://www.w3.org/2000/svg"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
	>
		<circle cx="12" cy="12" r="10" />
		<line x1="12" y1="8" x2="12" y2="12" />
		<line x1="12" y1="16" x2="12.01" y2="16" />
	</svg>
	<p>{message}</p>
</div>

<style>
	.error-message {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 16px;
		background-color: var(--error-bg);
		border: 1px solid var(--error-border);
		border-radius: 8px;
		margin-top: 16px;
	}

	.error-message svg {
		flex-shrink: 0;
		width: 24px;
		height: 24px;
		color: var(--error-icon);
	}

	.error-message p {
		margin: 0;
		color: var(--error-text);
		font-weight: 500;
		line-height: 1.5;
	}
</style>
