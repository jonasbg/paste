import { tr } from '$lib/i18n';
import { isTextBased } from '$lib/utils/mimeType';

export type FileMetadata = {
	filename?: string;
	contentType?: string;
	size?: number;
};

export const TEXT_PREVIEW_MAX_BYTES = 1024 * 1024;
export const TEXT_PREVIEW_MAX_CHARS = 120_000;
export const IMAGE_PREVIEW_MAX_BYTES = 10 * 1024 * 1024;

export const TEXT_PREVIEW_EXTENSIONS = new Set([
	'txt',
	'md',
	'markdown',
	'json',
	'jsonl',
	'log',
	'csv',
	'tsv',
	'xml',
	'yaml',
	'yml',
	'toml',
	'ini',
	'conf',
	'cfg',
	'env',
	'sh',
	'bash',
	'zsh',
	'fish',
	'ps1',
	'js',
	'ts',
	'jsx',
	'tsx',
	'css',
	'scss',
	'html',
	'htm',
	'svelte',
	'py',
	'rb',
	'php',
	'java',
	'go',
	'rs',
	'c',
	'cc',
	'cpp',
	'h',
	'hpp',
	'sql',
	'kt',
	'kts',
	'swift',
	'dart',
	'lua',
	'r',
	'rmd',
	'vue',
	'astro',
	'scala',
	'ex',
	'exs',
	'hs',
	'zig',
	'diff',
	'patch',
	'lock',
	'gradle',
	'pl',
	'pm',
	'properties',
	'rtf'
]);
export const IMAGE_PREVIEW_EXTENSIONS = new Set([
	'png',
	'jpg',
	'jpeg',
	'gif',
	'webp',
	'bmp',
	'avif',
	'svg'
]);
export const IMAGE_MIME_TYPES: Record<string, string> = {
	png: 'image/png',
	jpg: 'image/jpeg',
	jpeg: 'image/jpeg',
	gif: 'image/gif',
	webp: 'image/webp',
	bmp: 'image/bmp',
	avif: 'image/avif',
	svg: 'image/svg+xml'
};

function getFileExtension(filename: string | undefined): string {
	if (!filename || !filename.includes('.')) return '';
	return filename.split('.').pop()?.toLowerCase() || '';
}

export function isTextPreviewable(fileMetadata: FileMetadata | null | undefined): boolean {
	if (!fileMetadata?.filename) return false;

	// Trust the server-stored contentType (normalized at upload time)
	const contentType = fileMetadata.contentType?.toLowerCase() || '';
	if (contentType && isTextBased(contentType)) return true;

	// Extension fallback for files uploaded before MIME normalization
	return TEXT_PREVIEW_EXTENSIONS.has(getFileExtension(fileMetadata.filename));
}

export function isImagePreviewable(fileMetadata: FileMetadata | null | undefined): boolean {
	if (!fileMetadata?.filename) return false;

	const contentType = fileMetadata.contentType?.toLowerCase() || '';
	if (contentType.startsWith('image/')) return true;

	return IMAGE_PREVIEW_EXTENSIONS.has(getFileExtension(fileMetadata.filename));
}

export function resolveImagePreviewType(fileMetadata: FileMetadata): string | null {
	const contentType = fileMetadata.contentType?.toLowerCase() || '';
	if (contentType.startsWith('image/')) {
		return contentType;
	}

	return IMAGE_MIME_TYPES[getFileExtension(fileMetadata.filename)] || null;
}

export function formatPreviewLimit(bytes: number): string {
	if (bytes >= 1024 * 1024) {
		return `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
	}

	if (bytes >= 1024) {
		return `${(bytes / 1024).toFixed(0)} KB`;
	}

	return `${bytes} B`;
}

/**
 * Map a download error to the "file is gone" message when the server no longer
 * has the file (already downloaded, deleted or expired); null for other errors.
 */
export function getUnavailableDownloadMessage(error: unknown): string | null {
	const message = error instanceof Error ? error.message : String(error);
	if (message.includes('Failed to download file') || message.includes('Invalid access token')) {
		return tr('dl.unavailable');
	}
	return null;
}

export function formatEta(seconds: number): string {
	if (!isFinite(seconds) || seconds <= 0 || seconds > 3600) return '';
	if (seconds < 60) return `${Math.ceil(seconds)}s ${tr('common.remaining')}`;
	const mins = Math.floor(seconds / 60);
	const secs = Math.ceil(seconds % 60);
	return `${mins}m${secs > 0 ? ` ${secs}s` : ''} ${tr('common.remaining')}`;
}
