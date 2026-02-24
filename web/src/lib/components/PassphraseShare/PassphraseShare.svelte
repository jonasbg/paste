<script lang="ts">
	import { browser } from '$app/environment';

	export let passphrase: string = '';
	export let secureUrl: string = '';
	export let isVisible: boolean = false;

	$: passphraseLink = browser ? `${window.location.origin}/#passphrase=${passphrase}` : '';

	// Parse the secure URL into base + key parts
	$: secureBase = (() => {
		if (!secureUrl) return '';
		try {
			const u = new URL(secureUrl);
			return `${u.origin}${u.pathname}`;
		} catch {
			return '';
		}
	})();
	$: secureKey = (() => {
		if (!secureUrl) return '';
		try {
			const u = new URL(secureUrl);
			return new URLSearchParams(u.hash.slice(1)).get('key') ?? '';
		} catch {
			return '';
		}
	})();

	let copyMessage: string = '';
	let messageTimeout: number;

	function showMessage(message: string) {
		copyMessage = message;
		if (messageTimeout) clearTimeout(messageTimeout);
		messageTimeout = setTimeout(() => {
			copyMessage = '';
		}, 3000);
	}

	async function copy(text: string, label: string) {
		try {
			await navigator.clipboard.writeText(text);
			showMessage(label + ' kopiert!');
		} catch {
			const el = document.createElement('textarea');
			el.value = text;
			document.body.appendChild(el);
			el.select();
			document.execCommand('copy');
			document.body.removeChild(el);
			showMessage(label + ' kopiert!');
		}
	}
</script>

<div class="share-container" style="display: {isVisible ? 'block' : 'none'}">

	<!-- Option 1: Passphrase -->
	<div class="share-block">
		<div class="block-header">
			<h3>Del via delingskode</h3>
			<span class="badge badge-easy">Enklere å dele</span>
		</div>
		<p class="hint">En lesbar kode mottakeren skriver inn selv. Litt lavere entropi enn en tilfeldig nøkkel.</p>

		<div class="field-row">
			<label class="field-label">Lenke</label>
			<div class="input-group">
				<input type="text" class="url-field" value={passphraseLink} readonly />
				<button class="button" on:click={() => copy(passphraseLink, 'Lenke')}>Kopier</button>
			</div>
		</div>

		<div class="field-row">
			<label class="field-label">Kun kode</label>
			<div class="input-group">
				<input type="text" class="url-field" value={passphrase} readonly />
				<button class="button" on:click={() => copy(passphrase, 'Delingskode')}>Kopier</button>
			</div>
		</div>
	</div>

	<div class="separator"><span>eller</span></div>

	<!-- Option 2: Secure URL with key -->
	<div class="share-block">
		<div class="block-header">
			<h3>Del via sikker lenke</h3>
			<span class="badge badge-secure">Høyere sikkerhet</span>
		</div>
		<p class="hint">Tilfeldig kryptografisk nøkkel i URL-en. Kan ikke huskes — del hele lenken på én gang.</p>

		<div class="field-row">
			<label class="field-label">Komplett lenke</label>
			<div class="input-group">
				<input type="text" class="url-field" value={secureUrl} readonly />
				<button class="button" on:click={() => copy(secureUrl, 'Lenke')}>Kopier</button>
			</div>
		</div>

		<div class="field-row">
			<label class="field-label">Nettadresse</label>
			<div class="input-group">
				<input type="text" class="url-field" value={secureBase} readonly />
				<button class="button" on:click={() => copy(secureBase, 'Nettadresse')}>Kopier</button>
			</div>
		</div>

		<div class="field-row">
			<label class="field-label">Nøkkel</label>
			<div class="input-group">
				<input type="text" class="url-field" value={secureKey} readonly />
				<button class="button" on:click={() => copy(secureKey, 'Nøkkel')}>Kopier</button>
			</div>
		</div>
	</div>

	{#if copyMessage}
		<div class="copy-message">{copyMessage}</div>
	{/if}
</div>

<style>
	.share-container {
		margin-top: 1rem;
	}

	.share-block {
		background: #fff;
		padding: 1rem;
		border-radius: var(--border-radius);
		border: 1px solid #e0e0e0;
		margin-bottom: 0;
	}

	.block-header {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		margin-bottom: 0.4rem;
	}

	.block-header h3 {
		font-size: 1rem;
		margin: 0;
		font-weight: 500;
	}

	.badge {
		font-size: 0.7rem;
		font-weight: 600;
		padding: 0.15rem 0.5rem;
		border-radius: 99px;
		letter-spacing: 0.02em;
		text-transform: uppercase;
	}

	.badge-easy {
		background: rgba(var(--primary-green-rgb), 0.12);
		color: var(--primary-green);
	}

	.badge-secure {
		background: rgba(37, 99, 235, 0.1);
		color: #2563eb;
	}

	.hint {
		font-size: 0.8rem;
		color: #888;
		margin: 0 0 0.75rem 0;
		line-height: 1.4;
	}

	.field-row {
		margin-bottom: 0.5rem;
	}

	.field-row:last-child {
		margin-bottom: 0;
	}

	.field-label {
		display: block;
		font-size: 0.75rem;
		color: #666;
		margin-bottom: 0.25rem;
		font-weight: 500;
	}

	.input-group {
		display: flex;
		gap: 0.5rem;
	}

	.input-group .button {
		width: 7rem;
		flex-shrink: 0;
	}

	.url-field {
		flex: 1;
		padding: 0.625rem 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		font-family: inherit;
		background: #f5f5f5;
		font-size: 0.9rem;
		min-width: 0;
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

	.separator::before { left: 0; }
	.separator::after  { right: 0; }

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

	@media (max-width: 768px) {
		.input-group {
			flex-direction: column;
			gap: 0.5rem;
		}

		.input-group .button {
			width: 100%;
		}

		.url-field {
			font-size: 16px;
		}

		.copy-message {
			left: 1rem;
			right: 1rem;
		}
	}

	@keyframes slideIn {
		from { transform: translateY(100%); opacity: 0; }
		to   { transform: translateY(0);   opacity: 1; }
	}
</style>
