import { tr } from '$lib/i18n';
import { streamDownloadAndDecrypt } from '$lib/services/fileService';
import { generateHmacToken } from '$lib/utils/hmacUtils';

/** Outcome of the delete request sent after a successful download. */
export type DeleteOutcome = { deleted: true } | { deleted: false; message: string };

/**
 * Download, decrypt and save a file, then ask the server to delete it.
 *
 * Throws if the download or decryption fails (nothing was saved). Once the file
 * has been handed to the browser it never throws: a failed delete is reported
 * through the returned outcome, because the download itself succeeded and the
 * file is still on the server.
 */
export async function downloadSaveAndDelete(
	fileId: string,
	key: string,
	onProgress: (progress: number) => void
): Promise<DeleteOutcome> {
	const hmacToken = await generateHmacToken(fileId, key);

	const { stream, metadata } = await streamDownloadAndDecrypt(
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

	if (receivedLength === 0) {
		throw new Error(tr('dl.decryptError'));
	}

	const blob = new Blob(chunks, { type: metadata.contentType || 'application/octet-stream' });
	if (blob.size === 0) {
		throw new Error(tr('dl.decryptError'));
	}

	const url = window.URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = metadata.filename;
	document.body.appendChild(a);
	a.click();
	document.body.removeChild(a);
	window.URL.revokeObjectURL(url);

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
