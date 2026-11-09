import { tr } from '$lib/i18n';
import { downloadAndDecryptFile } from '$lib/services/fileService';
import { renderTextPreview } from '$lib/utils/textPreview';
import {
	type FileMetadata,
	IMAGE_PREVIEW_MAX_BYTES,
	TEXT_PREVIEW_MAX_BYTES,
	TEXT_PREVIEW_MAX_CHARS,
	formatPreviewLimit,
	getUnavailableDownloadMessage,
	isImagePreviewable,
	isTextPreviewable,
	resolveImagePreviewType
} from '$lib/utils/filePreview';

export type TextPreviewState =
	| { status: 'idle' }
	| { status: 'loading' }
	| { status: 'ready'; text: string; html: string; mode: 'pre' | 'table'; truncated: boolean }
	| { status: 'error'; message: string };

export type ImagePreviewState =
	| { status: 'idle' }
	| { status: 'loading' }
	| { status: 'ready'; url: string }
	| { status: 'error'; message: string };

export type CopyState = 'idle' | 'copied' | 'error';

type LoadArgs = {
	fileId: string;
	key: string;
	token: string;
	metadata: FileMetadata;
	/** Called when the server no longer has the file (already downloaded / deleted). */
	onUnavailable: (message: string) => void;
};

/**
 * Text and image preview state for one resolved file. Each load gets a
 * generation number; results from a superseded load are dropped, so a reset
 * (new key, new passphrase, page reset) can never be overwritten by a late
 * response from the previous file.
 */
export class FilePreviews {
	text = $state<TextPreviewState>({ status: 'idle' });
	image = $state<ImagePreviewState>({ status: 'idle' });
	copyState = $state<CopyState>('idle');

	#generation = 0;
	#copyTimer: ReturnType<typeof setTimeout> | null = null;

	/** Drop any in-flight load and return to idle. */
	reset() {
		this.#generation += 1;
		if (this.image.status === 'ready') URL.revokeObjectURL(this.image.url);
		this.text = { status: 'idle' };
		this.image = { status: 'idle' };
		this.#clearCopyTimer();
		this.copyState = 'idle';
	}

	/** Reset, then load whichever previews the file type supports. */
	async load(args: LoadArgs): Promise<void> {
		this.reset();
		const generation = this.#generation;
		const tasks: Promise<void>[] = [];
		if (isTextPreviewable(args.metadata)) tasks.push(this.#loadText(args, generation));
		if (isImagePreviewable(args.metadata)) tasks.push(this.#loadImage(args, generation));
		await Promise.all(tasks);
	}

	async copyText(): Promise<void> {
		if (this.text.status !== 'ready') return;
		const text = this.text.text;
		try {
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(text);
				this.copyState = 'copied';
			} else {
				// Fallback: use a temporary textarea for older browsers / insecure contexts
				const textarea = document.createElement('textarea');
				textarea.value = text;
				textarea.style.position = 'fixed';
				textarea.style.left = '-9999px';
				textarea.style.top = '-9999px';
				document.body.appendChild(textarea);
				textarea.focus();
				textarea.select();
				const success = document.execCommand('copy');
				document.body.removeChild(textarea);
				this.copyState = success ? 'copied' : 'error';
			}
		} catch {
			this.copyState = 'error';
		}
		this.#clearCopyTimer();
		this.#copyTimer = setTimeout(() => {
			this.copyState = 'idle';
			this.#copyTimer = null;
		}, 1800);
	}

	#clearCopyTimer() {
		if (this.#copyTimer) clearTimeout(this.#copyTimer);
		this.#copyTimer = null;
	}

	async #loadText(args: LoadArgs, generation: number) {
		if ((args.metadata.size || 0) > TEXT_PREVIEW_MAX_BYTES) {
			this.text = {
				status: 'error',
				message: tr('preview.textOnlyLimit', { limit: formatPreviewLimit(TEXT_PREVIEW_MAX_BYTES) })
			};
			return;
		}

		this.text = { status: 'loading' };
		try {
			const { decrypted } = await downloadAndDecryptFile(
				args.fileId,
				args.key,
				args.token,
				async () => {}
			);
			const previewText = (await decrypted.text()).replace(/\r\n/g, '\n');
			if (generation !== this.#generation) return;

			const clipped = previewText.slice(0, TEXT_PREVIEW_MAX_CHARS);
			const rendered = renderTextPreview(clipped, args.metadata);
			this.text = {
				status: 'ready',
				text: clipped,
				html: rendered.html,
				mode: rendered.mode,
				truncated: previewText.length > TEXT_PREVIEW_MAX_CHARS
			};
		} catch (error) {
			if (generation !== this.#generation) return;
			console.error('Text preview error:', error);
			const unavailable = getUnavailableDownloadMessage(error);
			if (unavailable) {
				this.text = { status: 'idle' };
				args.onUnavailable(unavailable);
				return;
			}
			this.text = { status: 'error', message: tr('preview.textLoadError') };
		}
	}

	async #loadImage(args: LoadArgs, generation: number) {
		if ((args.metadata.size || 0) > IMAGE_PREVIEW_MAX_BYTES) {
			this.image = {
				status: 'error',
				message: tr('preview.imageOnlyLimit', { limit: formatPreviewLimit(IMAGE_PREVIEW_MAX_BYTES) })
			};
			return;
		}

		this.image = { status: 'loading' };
		try {
			const { decrypted } = await downloadAndDecryptFile(
				args.fileId,
				args.key,
				args.token,
				async () => {}
			);
			const previewType = resolveImagePreviewType(args.metadata);
			const blob =
				previewType && decrypted.type !== previewType
					? new Blob([decrypted], { type: previewType })
					: decrypted;
			if (generation !== this.#generation) return;
			this.image = { status: 'ready', url: URL.createObjectURL(blob) };
		} catch (error) {
			if (generation !== this.#generation) return;
			console.error('Image preview error:', error);
			const unavailable = getUnavailableDownloadMessage(error);
			if (unavailable) {
				this.image = { status: 'idle' };
				args.onUnavailable(unavailable);
				return;
			}
			this.image = { status: 'error', message: tr('preview.imageLoadError') };
		}
	}
}
