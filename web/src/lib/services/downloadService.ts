import { tr } from '$lib/i18n';
import { streamDownloadAndDecrypt, type RemainingDownloads } from '$lib/services/fileService';
import { generateHmacToken } from '$lib/utils/hmacUtils';

/** Outcome of the download + server retention update. */
export type DeleteOutcome =
	| { deleted: true }
	| { deleted: false; remaining: number | 'unlimited'; maxDownloads: number | null }
	| { deleted: false; message: string };

/**
 * Download, decrypt and save a file.
 *
 * Servers supporting download limits report X-Downloads-Remaining on GET /api/download/:id
 * and delete the file automatically when remaining reaches 0. When talking to an older
 * server without that header, falls back to DELETE /api/delete/:id.
 */
export async function downloadSaveAndDelete(
	fileId: string,
	key: string,
	onProgress: (progress: number) => void
): Promise<DeleteOutcome> {
	const hmacToken = await generateHmacToken(fileId, key);

	const { stream, metadata, maxDownloads, remainingDownloads } = await streamDownloadAndDecrypt(
		fileId,
		key,
		hmacToken,
		async (progress) => onProgress(progress)
	);

	const reader = stream.getReader();
	const chunks: BlobPart[] = [];
	let receivedLength = 0;

	while (true) {
		const { done, value } = await reader.read();
		if (done) break;
		if (value) {
			chunks.push(new Uint8Array(value));
			receivedLength += value.length;
		}
	}

	if (receivedLength === 0 && (metadata.size ?? 0) > 0) {
		throw new Error(tr('dl.decryptError'));
	}

	const blob = new Blob(chunks, { type: metadata.contentType || 'application/octet-stream' });

	const url = window.URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = metadata.filename;
	document.body.appendChild(a);
	a.click();
	document.body.removeChild(a);
	setTimeout(() => window.URL.revokeObjectURL(url), 1000);

	// Server-managed download count: already decremented/deleted on GET /api/download/:id.
	if (remainingDownloads !== null) {
		if (remainingDownloads === 0) {
			return { deleted: true };
		}
		return {
			deleted: false,
			remaining: remainingDownloads,
			maxDownloads
		};
	}

	// Legacy server fallback: explicitly delete after download.
	try {
		const response = await fetch(`/api/delete/${fileId}`, {
			method: 'DELETE',
			headers: {
				'Content-Type': 'application/json',
				'X-HMAC-Token': hmacToken
			}
		});
		return response.ok ? { deleted: true } : { deleted: false, message: tr('dl.deleteFailed') };
	} catch (error) {
		console.error('Delete error:', error);
		return { deleted: false, message: tr('dl.deleteNetworkError') };
	}
}

export type { RemainingDownloads };
