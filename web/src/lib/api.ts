export async function uploadFile(formData: FormData): Promise<{ id: string }> {
	const response = await fetch('/api/upload', {
		method: 'POST',
		body: formData
	});

	if (!response.ok) {
		const error = await response.json();
		throw new Error(error.message || 'Upload failed');
	}

	return response.json();
}

export async function downloadFile(fileId: string): Promise<ArrayBuffer> {
	const response = await fetch(`/api/download/${fileId}`);

	if (!response.ok) {
		const error = await response.json();
		throw new Error(error.message || 'Download failed');
	}

	return response.arrayBuffer();
}

interface MetadataResponse {
	buffer: ArrayBuffer;
	size?: number;
}

function formatFileSize(bytes: number | undefined): string {
	if (!bytes) return '';

	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	let size = bytes;
	let unitIndex = 0;

	while (size >= 1024 && unitIndex < units.length - 1) {
			size /= 1024;
			unitIndex++;
	}

	return `${size.toFixed(1)} ${units[unitIndex]}`;
}

export async function getFileMetadata(fileId: string): Promise<MetadataResponse> {
	const response = await fetch(`/api/metadata/${fileId}`);

	if (!response.ok) {
			const error = await response.json();
			throw new Error(error.message || 'Failed to get metadata');
	}

	const fileSize = response.headers.get('X-File-Size');
	const arrayBuffer = await response.arrayBuffer();

	return {
			buffer: arrayBuffer,
			size: formatFileSize(fileSize ? parseInt(fileSize, 10) : undefined)
	};
}
