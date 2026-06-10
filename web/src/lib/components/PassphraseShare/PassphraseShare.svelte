<script lang="ts">
	import { browser } from '$lib/env';
	import { t, tr } from '$lib/i18n';

	interface Props {
		passphrase?: string;
		secureUrl?: string;
		isVisible?: boolean;
	}

	let { passphrase = '', secureUrl = '', isVisible = false }: Props = $props();

	let passphraseLink = $derived(browser ? `${window.location.origin}/#passphrase=${passphrase}` : '');

	// Parse the secure URL into base + key parts
	let secureBase = $derived((() => {
		if (!secureUrl) return '';
		try {
			const u = new URL(secureUrl);
			return `${u.origin}${u.pathname}`;
		} catch {
			return '';
		}
	})());
	let secureKey = $derived((() => {
		if (!secureUrl) return '';
		try {
			const u = new URL(secureUrl);
			return new URLSearchParams(u.hash.slice(1)).get('key') ?? '';
		} catch {
			return '';
		}
	})());

	let copyMessage: string = $state('');
	let messageTimeout: number;

	function fitScale(node: HTMLElement) {
		let frame = 0;
		let resizeObserver: ResizeObserver | null = null;

		const update = () => {
			frame = 0;
			const viewportHeight = window.visualViewport?.height || window.innerHeight;
			const rect = node.getBoundingClientRect();
			const availableHeight = Math.max(220, Math.floor(viewportHeight - rect.top - 24));
			const naturalHeight = node.scrollHeight;
			const scale = Math.min(1, availableHeight / Math.max(naturalHeight, 1));
			const shell = node.parentElement;

			node.style.transformOrigin = 'top center';
			node.style.transform = `scale(${scale})`;

			if (shell) {
				shell.style.height = `${Math.ceil(naturalHeight * scale)}px`;
			}
		};

		const scheduleUpdate = () => {
			if (frame) cancelAnimationFrame(frame);
			frame = requestAnimationFrame(update);
		};

		scheduleUpdate();
		window.addEventListener('resize', scheduleUpdate);
		window.visualViewport?.addEventListener('resize', scheduleUpdate);

		if ('ResizeObserver' in window) {
			resizeObserver = new ResizeObserver(scheduleUpdate);
			resizeObserver.observe(node);
		}

		return {
			update: scheduleUpdate,
			destroy() {
				if (frame) cancelAnimationFrame(frame);
				window.removeEventListener('resize', scheduleUpdate);
				window.visualViewport?.removeEventListener('resize', scheduleUpdate);
				resizeObserver?.disconnect();
			}
		};
	}

	function showMessage(message: string) {
		copyMessage = message;
		if (messageTimeout) clearTimeout(messageTimeout);
		messageTimeout = setTimeout(() => {
			copyMessage = '';
		}, 3000);
	}

	async function copy(text: string, labelKey: string) {
		const message = tr('share.copiedToast', { label: tr(labelKey) });
		try {
			await navigator.clipboard.writeText(text);
			showMessage(message);
		} catch {
			const el = document.createElement('textarea');
			el.value = text;
			document.body.appendChild(el);
			el.select();
			document.execCommand('copy');
			document.body.removeChild(el);
			showMessage(message);
		}
	}
</script>

<div class="share-shell" style="display: {isVisible ? 'block' : 'none'}">
	<div class="share-container" use:fitScale>
		<!-- Option 1: Passphrase -->
		<div class="share-block">
			<div class="block-header">
				<h3>{$t('share.viaCode')}</h3>
				<span class="badge badge-easy">{$t('share.easierBadge')}</span>
			</div>
			<p class="hint">{$t('share.codeHint')}</p>

			<div class="field-row">
				<label class="field-label">{$t('share.link')}</label>
				<div class="input-group">
					<input type="text" class="url-field" value={passphraseLink} readonly />
					<button class="button" onclick={() => copy(passphraseLink, 'share.link')}>{$t('common.copy')}</button>
				</div>
			</div>

			<div class="field-row">
				<label class="field-label">{$t('share.codeOnly')}</label>
				<div class="input-group">
					<input type="text" class="url-field" value={passphrase} readonly />
					<button class="button" onclick={() => copy(passphrase, 'share.labelCode')}>{$t('common.copy')}</button>
				</div>
			</div>
		</div>

		<div class="separator"><span>{$t('common.or')}</span></div>

		<!-- Option 2: Secure URL with key -->
		<div class="share-block">
			<div class="block-header">
				<h3>{$t('share.viaSecureLink')}</h3>
				<span class="badge badge-secure">{$t('share.higherSecurityBadge')}</span>
			</div>
			<p class="hint">{$t('share.secureHint')}</p>

			<div class="field-row">
				<label class="field-label">{$t('share.completeLink')}</label>
				<div class="input-group">
					<input type="text" class="url-field" value={secureUrl} readonly />
					<button class="button" onclick={() => copy(secureUrl, 'share.link')}>{$t('common.copy')}</button>
				</div>
			</div>

			<hr class="field-divider" />

			<div class="field-row">
				<label class="field-label">{$t('share.webAddress')}</label>
				<div class="input-group">
					<input type="text" class="url-field" value={secureBase} readonly />
					<button class="button" onclick={() => copy(secureBase, 'share.webAddress')}>{$t('common.copy')}</button>
				</div>
			</div>

			<div class="field-row">
				<label class="field-label">{$t('share.key')}</label>
				<div class="input-group">
					<input type="text" class="url-field" value={secureKey} readonly />
					<button class="button" onclick={() => copy(secureKey, 'share.key')}>{$t('common.copy')}</button>
				</div>
			</div>
		</div>

		{#if copyMessage}
			<div class="copy-message">{copyMessage}</div>
		{/if}
	</div>
</div>

<style>
	.share-shell {
		margin-top: 1rem;
		overflow: hidden;
	}

	.share-container {
		will-change: transform;
	}

	.share-block {
		background: #fff;
		padding: 0.875rem;
		border-radius: var(--border-radius);
		border: 1px solid #e0e0e0;
		margin-bottom: 0;
	}

	.block-header {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.6rem;
		margin-bottom: 0.3rem;
	}

	.block-header h3 {
		font-size: 0.95rem;
		margin: 0;
		font-weight: 500;
	}

	.badge {
		font-size: 0.7rem;
		font-weight: 600;
		padding: 0.2rem 0.6rem;
		border-radius: 99px;
		letter-spacing: 0.03em;
		text-transform: uppercase;
		border: 1px solid transparent;
	}

	.badge-easy {
		background: #bbf7d0;
		color: #065f46;
		border-color: #6ee7b7;
	}

	.badge-secure {
		background: #bfdbfe;
		color: #1e40af;
		border-color: #93c5fd;
	}

	.hint {
		font-size: 0.75rem;
		color: #888;
		margin: 0 0 0.6rem 0;
		line-height: 1.4;
	}

	.field-divider {
		border: none;
		border-top: 1px solid #e0e0e0;
		margin: 0.85rem 0;
	}

	.field-row {
		margin-bottom: 0.7rem;
	}

	.field-row:last-child {
		margin-bottom: 0;
	}

	.field-label {
		display: block;
		font-size: 0.7rem;
		color: #666;
		margin-bottom: 0.2rem;
		font-weight: 500;
	}

	.input-group {
		display: flex;
		gap: 0.45rem;
	}

	.input-group .button {
		width: 6.25rem;
		flex-shrink: 0;
		padding: 0.55rem 0.7rem;
		font-size: 0.82rem;
	}

	.url-field {
		flex: 1;
		padding: 0.55rem 0.65rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		font-family: inherit;
		background: #f5f5f5;
		font-size: 0.82rem;
		min-width: 0;
	}

	.separator {
		text-align: center;
		margin: 0.75rem 0;
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
		font-size: 0.78rem;
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
		.share-shell {
			overflow: visible;
			height: auto !important;
		}

		.share-container {
			transform: none !important;
		}

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
