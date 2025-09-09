<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { streamDownloadAndDecrypt, fetchMetadata } from '$lib/services/fileService';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import SuccessMessage from '$lib/components/SuccessMessage.svelte';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import FileInfo from '$lib/components/FileUpload/FileInfo.svelte';
	import { replaceState } from '$app/navigation';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import { generateHmacToken } from '$lib/utils/hmacUtils';

	let encryptionKey: string = '';
	let manualKeyInput: string = '';
	let metadata: any = null;
	let fileSize: string | undefined;
	let downloadProgress = 0;
	let downloadMessage = '';
	let isDownloading = false;
	let downloadError: string | null = null;
	let isDownloadComplete = false;
	let keyError: string | null = null;
	let isLoading = true;
	let deletionError: string | null = null;

	// Function to validate and extract key from input
	function validateAndExtractKey(input: string): string | null {
		input = input.trim();

		if (input.includes('://')) {
			try {
				const url = new URL(input);
				const hashParams = new URLSearchParams(url.hash.slice(1));
				return hashParams.get('key');
			} catch (error) {
				return null;
			}
		}

		const base64Regex = /^[A-Za-z0-9+/=_-]+$/;
		if (base64Regex.test(input)) {
			return input;
		}

		return null;
	}

	async function getMetadata() {
		try {
			// Ensure WASM runtime is initialized before HMAC generation or decryption
			const { initWasm } = await import('$lib/utils/wasm-loader');
			await initWasm();
			const fileId = $page.params.fileId;
			// Generate token from encryption key
			const hmacToken = await generateHmacToken(fileId, encryptionKey);

			const metadataResponse = await fetchMetadata(fileId, encryptionKey, hmacToken);
			metadata = metadataResponse.metadata;
			fileSize = metadataResponse.size?.toString();
		} catch (error) {
			console.error('Metadata error:', error);
			encryptionKey = '';
			manualKeyInput = '';
			metadata = {
				error:
					'Kunne ikke hente filinformasjon. Sjekk at nøkkelen er riktig, eller at filen ikke er slettet.'
			};
		} finally {
			isLoading = false;
		}
	}

	// Function to safely handle encryption key without exposing it in URL
	function setEncryptionKey(key: string) {
		encryptionKey = key;
		// Clear any sensitive data from URL without adding to history
		if (browser) {
			replaceState('', window.location.pathname);
		}
	}

	async function handleManualKeySubmit() {
		if (!manualKeyInput.trim()) return;

		const key = validateAndExtractKey(manualKeyInput.trim());
		if (key) {
			setEncryptionKey(key);
			await getMetadata();
		} else {
			keyError = 'Invalid key or URL';
		}
	}

	async function initiateDownload() {
    if (!encryptionKey || isDownloading || !metadata || metadata.error) return; // Prevent download if metadata failed
    isDownloading = true;
    downloadError = null;

    try {
        const fileId = $page.params.fileId;
        const hmacToken = await generateHmacToken(fileId, encryptionKey);

        // Use streamDownloadAndDecrypt instead of downloadAndDecryptFile
        const { stream, metadata: fileMetadata } = await streamDownloadAndDecrypt(
            fileId,
            encryptionKey,
            hmacToken,
            async (progress, message) => {
                downloadProgress = progress;
                downloadMessage = message;
            }
        );

        // Read the stream and collect chunks
        const reader = stream.getReader();
        const chunks = [];
        let receivedLength = 0;

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            if (value) {
                chunks.push(value);
                receivedLength += value.length;
            }
        }

        // Check if any data was received
        if (receivedLength === 0) {
            throw new Error('Kunne ikke dekryptere filen - filen er nå slettet fra serveren');
        }

        // Create a Blob from the collected chunks
        const blob = new Blob(chunks, {
            type: fileMetadata.contentType || 'application/octet-stream'
        });

        // Verify the Blob has content
        if (blob.size === 0) {
            throw new Error('Kunne ikke dekryptere filen - filen er nå slettet fra serveren');
        }

        // Create and trigger the download
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = fileMetadata.filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        window.URL.revokeObjectURL(url);

        // Attempt to delete the file from the server
        try {
            const deleteResponse = await fetch(`/api/delete/${fileId}`, {
                method: 'DELETE',
                headers: {
                    'Content-Type': 'application/json',
                    'X-HMAC-Token': hmacToken
                }
            });

            // Clear sensitive data
            encryptionKey = '';
            manualKeyInput = '';

            // Check if deletion was successful
            if (deleteResponse.ok) {
                isDownloadComplete = true;
                deletionError = null;
            } else {
                deletionError = 'Filen ble lastet ned, men kunne ikke slettes fra serveren.';
                console.error('Error deleting file:', deleteResponse.status, deleteResponse.statusText);
            }
        } catch (err) {
            console.error('Error deleting file:', err);
            deletionError =
                'Filen ble lastet ned, men kunne ikke slettes fra serveren på grunn av en nettverksfeil.';
        }

        // Clean the URL in the browser
        if (browser) {
            window.history.replaceState({}, '', '/');
        }
    } catch (error) {
        console.error('Download error:', error);
        downloadError = (error as Error).message;
        downloadProgress = 0;
        downloadMessage = '';
    } finally {
        isDownloading = false;
    }
}

	onMount(async () => {
		if (!browser) return;

		try {
			const { initWasm } = await import('$lib/utils/wasm-loader');
			await initWasm();
			if (window.location.hash) {
				// Check if a hash exists
				const urlParams = new URLSearchParams(window.location.hash.slice(1));
				const key = urlParams.get('key');
				if (key) {
					const validatedKey = validateAndExtractKey(key);
					if (validatedKey) {
						setEncryptionKey(validatedKey);
						await getMetadata(); // Get metadata on mount as well
					} else {
						keyError = 'Ugyldig nøkkel eller URL'; // More generic
					}
				}
			}
		} catch (error) {
			console.error('Failed to initialize:', error);
			downloadError = 'Failed to initialize the application';
		} finally {
			isLoading = false;
		}
	});

	$: canDownload = !!(
		metadata &&
		!metadata.error && // Ensure metadata is available and has no errors
		!isDownloading &&
		!isDownloadComplete &&
		encryptionKey
	);

	// Reset keyError whenever manualKeyInput changes
	$: manualKeyInput, (keyError = null);
</script>

<div class="container">
	<div class="download-container">
		<h1><span>Sikker</span> fildeling</h1>
		<p class="intro-text">
			Velkommen til vår sikre fildelingstjeneste. Her kan du trygt laste ned filer som har blitt
			delt med deg. Alle filer er ende-til-ende-kryptert, som betyr at bare du med riktig
			dekrypteringsnøkkel kan få tilgang til innholdet. Etter vellykket nedlasting blir filen
			automatisk slettet fra våre servere.
		</p>

		{#if isLoading}
			<LoadingSpinner message="" />
		{/if}

		{#if downloadError}
			<ErrorMessage message={downloadError} />
		{:else if metadata?.error}
			<ErrorMessage message={metadata.error} />
		{:else}
			{#if metadata?.filename}
				<FileInfo fileName={metadata.filename} {fileSize} />
				{#if !isDownloadComplete}
					{#if !isDownloading}
						<button class="button" on:click={initiateDownload} disabled={!canDownload}>
							{isDownloading ? 'Laster ned...' : 'Last ned'}
						</button>
					{/if}
				{/if}
			{/if}

			{#if isDownloading || isDownloadComplete}
				<ProgressBar
					progress={downloadProgress}
					message={downloadMessage}
					isVisible={isDownloading}
				/>
			{/if}

			{#if isDownloadComplete}
				{#if deletionError}
					<ErrorMessage message={deletionError} />
				{:else}
					<SuccessMessage
						message="Filen er lastet ned og sikkert slettet fra serveren vår. Takk for at du bruker vår sikre fildelingstjeneste!"
					/>
				{/if}
			{/if}
		{/if}

		{#if !encryptionKey && !isDownloadComplete && !isLoading}
			<form class="key-prompt" on:submit|preventDefault={handleManualKeySubmit}>
				<h2>Dekrypteringsnøkkel kreves</h2>
				<p>
					Du trenger en dekrypteringsnøkkel for å få tilgang til denne filen. Vennligst lim inn hele
					lenken du har mottatt, så vil nøkkelen automatisk bli hentet ut. Alternativt kan du lime
					inn dekrypteringsnøkkelen direkte under.
				</p>
				<div class="input-group">
					<input
						type="text"
						class="key-input"
						placeholder="Lim inn dekrypteringsnøkkel eller hele URL-en"
						bind:value={manualKeyInput}
						disabled={isDownloading}
					/>
					<button
						type="submit"
						class="decrypt-button"
						disabled={!manualKeyInput.trim() || isDownloading}
					>
						Dekrypter
					</button>
				</div>
				{#if keyError}
					<div class="error-message">
						{keyError}
					</div>
				{/if}
			</form>
		{/if}
	</div>
</div>

<style>
	.download-container {
		border-radius: var(--border-radius);
		padding: 2rem;
	}

	h1 {
		font-size: 2.5rem;
		font-weight: 500;
	}

	h1 span {
		color: var(--primary-green);
	}

	.key-prompt {
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 2rem;
		margin-top: 2rem;
	}

	.key-prompt h2 {
		font-size: 1.25rem;
		margin: 0 0 1rem 0;
		font-weight: 500;
	}

	.key-prompt p {
		margin-bottom: 1rem;
		color: #666;
	}

	.key-input {
		width: 100%;
		padding: 0.75rem;
		border: 1px solid #e0e0e0;
		border-radius: var(--border-radius);
		font-family: inherit;
	}

	.key-input:focus {
		outline: none;
		border-color: var(--primary-green);
		box-shadow: 0 0 0 2px rgba(64, 184, 123, 0.2);
	}

	.input-group {
		display: flex;
		gap: 0.5rem;
	}

	.decrypt-button {
		white-space: nowrap;
		padding: 0.75rem 1.5rem;
		background-color: var(--primary-green);
		color: white;
		border: none;
		border-radius: var(--border-radius);
		cursor: pointer;
		font-weight: 500;
	}

	.decrypt-button:disabled {
		background-color: #ccc;
		cursor: not-allowed;
	}

	.error-message {
		color: var(--error-red, #dc3545);
		margin-top: 0.5rem;
		font-size: 0.875rem;
	}
	.intro-text {
		color: #666;
		margin: 1rem 0 2rem 0;
		line-height: 1.5;
		max-width: 800px;
	}
</style>
