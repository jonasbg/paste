<script lang="ts">
	export let passphrase: string = '';
	export let isVisible: boolean = false;

	let copyMessage: string = '';
	let messageTimeout: number;

	function showMessage(message: string) {
		copyMessage = message;
		if (messageTimeout) clearTimeout(messageTimeout);
		messageTimeout = setTimeout(() => {
			copyMessage = '';
		}, 3000);
	}

	async function copyPassphrase() {
		try {
			await navigator.clipboard.writeText(passphrase);
			showMessage('Løsenord kopiert!');
		} catch {
			const textArea = document.createElement('textarea');
			textArea.value = passphrase;
			document.body.appendChild(textArea);
			textArea.select();
			document.execCommand('copy');
			document.body.removeChild(textArea);
			showMessage('Løsenord kopiert!');
		}
	}
</script>

<div class="passphrase-container" style="display: {isVisible ? 'block' : 'none'}">
	<div class="passphrase-section">
		<h3>Løsenord for deling</h3>
		<p class="hint">
			Del dette løsenordet med mottakeren. De kan skrive det inn på forsiden for å laste ned filen.
		</p>
		<div class="input-group">
			<input type="text" class="passphrase-field" value={passphrase} readonly />
			<button class="button" on:click={copyPassphrase}>Kopier løsenord</button>
		</div>
	</div>

	<div class="info-box">
		<p>
			Mottakeren skriver inn løsenordet i feltet "Har du et løsenord?" på forsiden for å laste ned
			filen.
		</p>
	</div>

	{#if copyMessage}
		<div class="copy-message">{copyMessage}</div>
	{/if}
</div>

<style>
	.passphrase-container {
		margin-top: 1rem;
	}

	.passphrase-section {
		background: #fff;
		padding: 1rem;
		border-radius: var(--border-radius);
		border: 1px solid #e0e0e0;
		margin-bottom: 1rem;
	}

	.passphrase-section h3 {
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

	.passphrase-field {
		flex: 1;
		padding: 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		font-family: inherit;
		background: #f5f5f5;
		font-size: 1rem;
		letter-spacing: 0.02em;
	}

	.info-box {
		background: rgba(var(--primary-green-rgb), 0.05);
		border: 1px solid rgba(var(--primary-green-rgb), 0.2);
		border-radius: var(--border-radius);
		padding: 0.75rem 1rem;
		font-size: 0.875rem;
		color: #444;
	}

	.info-box p {
		margin: 0;
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

		.passphrase-field {
			font-size: 16px;
		}

		.passphrase-section {
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
