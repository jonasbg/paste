import { getWasmInstance } from '$lib/utils/wasm-loader';
import { FileProcessor } from './fileProcessor';

export function generateKey(): string | null {
	const wasmInstance = getWasmInstance();
	if (!wasmInstance) return null;

	const key = wasmInstance.generateKey();
	return key;
}

export async function uploadEncryptedFile(
	file: File,
	key: string,
	onProgress: (progress: number, message: string) => Promise<void>
): Promise<string> {
	const processor = new FileProcessor();

	// Yield to event loop before starting
	await new Promise((resolve) => setTimeout(resolve, 0));

	const { header, encryptedContent } = await processor.encryptFile(file, key, onProgress);

	// Yield to event loop before upload progress
	await new Promise((resolve) => setTimeout(resolve, 0));
	await onProgress(50, 'Laster opp...');

	return new Promise((resolve, reject) => {
		const xhr = new XMLHttpRequest();

		xhr.open('POST', '/api/upload', true);

		// Track upload progress
		xhr.upload.onprogress = async (event) => {
			if (event.lengthComputable) {
				const percentComplete = Math.round((event.loaded / event.total) * 50) + 50;
				await onProgress(
					percentComplete,
					`Laster opp... (${Math.round((event.loaded / event.total) * 100)}%)`
				);
			}
		};

		xhr.onload = async () => {
			if (xhr.status === 200) {
				try {
					const result = JSON.parse(xhr.responseText);
					await onProgress(100, 'Fullført!');
					resolve(result.id);
				} catch (error) {
					reject(new Error('Kunne ikke tolke server-respons'));
				}
			} else {
				reject(new Error('Opplasting feilet'));
			}
		};

		xhr.onerror = () => {
			reject(new Error('Nettverksfeil under opplasting'));
		};

		// Prepare FormData
		const formData = new FormData();
		const blob = new Blob([header, encryptedContent]);
		formData.append('file', blob, 'encrypted_container');

		// Send the request
		xhr.send(formData);
	});
}
