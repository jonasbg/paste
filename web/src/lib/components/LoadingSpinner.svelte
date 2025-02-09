<script lang="ts">
	export let message = 'Henter filinformasjon...';
</script>

<div class="loading-container">
	<div class="spinner"></div>
	<p>{message}</p>
</div>

<style>
	.loading-container {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 2rem;
	}

	.spinner {
		width: 48px;
		height: 48px;
		border: 4px solid #e0e0e0;
		border-top: 4px solid var(--primary-green);
		border-radius: 50%;
		animation: spin 1s linear infinite;
		margin-bottom: 1rem;
	}

	@keyframes spin {
		0% {
			transform: rotate(0deg);
		}
		100% {
			transform: rotate(360deg);
		}
	}

	p {
		color: #666;
		font-size: 0.875rem;
	}
</style>
