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
