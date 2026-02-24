<script lang="ts">
	import { browser } from '$app/environment';

	export let passphrase: string = '';
	export let isVisible: boolean = false;

	$: shareUrl = browser ? `${window.location.origin}/#passphrase=${passphrase}` : '';

	let copyMessage: string = '';
	let messageTimeout: number;

	function showMessage(message: string) {
		copyMessage = message;
		if (messageTimeout) clearTimeout(messageTimeout);
		messageTimeout = setTimeout(() => {
			copyMessage = '';
		}, 3000);
	}

	async function copyToClipboard(text: string, label: string) {
		try {
			await navigator.clipboard.writeText(text);
			showMessage(label + ' kopiert!');
		} catch {
			const textArea = document.createElement('textarea');
			textArea.value = text;
			document.body.appendChild(textArea);
			textArea.select();
			document.execCommand('copy');
			document.body.removeChild(textArea);
			showMessage(label + ' kopiert!');
		}
	}
</script>

<div class="url-container" style="display: {isVisible ? 'block' : 'none'}">
	<div class="copy-section">
		<h3>Del via lenke</h3>
		<p class="hint">Send denne lenken direkte — mottakeren klikker den og nedlastingen starter automatisk.</p>
		<div class="input-group">
			<input type="text" class="url-field" value={shareUrl} readonly />
			<button class="button" on:click={() => copyToClipboard(shareUrl, 'Lenke')}>Kopier lenke</button>
		</div>
	</div>

	<div class="separator">
		<span>eller</span>
	</div>

	<div class="copy-section">
		<h3>Delingskode</h3>
		<p class="hint">Del kun koden — mottakeren skriver den inn på forsiden for å laste ned filen.</p>
		<div class="input-group">
			<input type="text" class="url-field" value={passphrase} readonly />
			<button class="button" on:click={() => copyToClipboard(passphrase, 'Delingskode')}>Kopier kode</button>
		</div>
	</div>

	{#if copyMessage}
		<div class="copy-message">{copyMessage}</div>
	{/if}
</div>

<style>
	.url-container {
		margin-top: 1rem;
	}

	.copy-section {
		background: #fff;
		padding: 1rem;
		border-radius: var(--border-radius);
		border: 1px solid #e0e0e0;
		margin-bottom: 1rem;
	}

	.copy-section h3 {
		font-size: 1rem;
		margin: 0 0 0.5rem 0;
		font-weight: 500;
	}

	.hint {
		font-size: 0.875rem;
		color: #666;
		margin-bottom: 0.75rem;
	}

	.input-group {
		display: flex;
		gap: 0.5rem;
	}

	.input-group .button {
		width: 13rem;
		flex-shrink: 0;
	}

	.url-field {
		flex: 1;
		padding: 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		font-family: inherit;
		background: #f5f5f5;
		font-size: 1rem;
	}

	.separator {
		text-align: center;
		margin: 1rem 0;
		position: relative;
	}

	.separator::before,
	.separator::after {
		content: '';
		position: absolute;
		top: 50%;
		width: calc(50% - 2rem);
		height: 1px;
		background: #e0e0e0;
	}

	.separator::before {
		left: 0;
	}

	.separator::after {
		right: 0;
	}

	.separator span {
		background: var(--background-color);
		padding: 0 1rem;
		color: #666;
		font-size: 0.875rem;
	}

	.copy-message {
		position: fixed;
		bottom: 1rem;
		right: 1rem;
		background: var(--primary-green);
		color: white;
		padding: 0.75rem 1.5rem;
		border-radius: var(--border-radius);
		animation: slideIn 0.3s ease-out;
		z-index: 999999;
	}

	/* Responsive styles */
	@media (max-width: 768px) {
		.input-group {
			flex-direction: column;
			gap: 0.75rem;
		}

		.input-group .button {
			width: 100%;
		}

		.url-field {
			font-size: 16px;
		}

		.copy-section {
			padding: 0.75rem;
		}

		.copy-message {
			left: 1rem;
			right: 1rem;
			bottom: 1rem;
		}
	}

	@keyframes slideIn {
		from {
			transform: translateY(100%);
			opacity: 0;
		}
		to {
			transform: translateY(0);
			opacity: 1;
		}
	}
</style>
