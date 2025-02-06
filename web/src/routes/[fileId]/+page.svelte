<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { initWasm } from '$lib/utils/wasm-loader';
	import { downloadAndDecryptFile, fetchMetadata } from '$lib/services/fileService';
	import ErrorMessage from '$lib/components/ErrorMessage.svelte';
	import SuccessMessage from '$lib/components/SuccessMessage.svelte';
	import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
	import FileInfo from '$lib/components/FileUpload/FileInfo.svelte';
	import { replaceState } from '$app/navigation';
	import { getFileMetadata } from '$lib/api';

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

	// Function to validate and extract key from input
	function validateAndExtractKey(input: string): string | null {
		// Remove any whitespace
		input = input.trim();

		// If it's a URL, try to extract the key
		if (input.includes('://')) {
			try {
				const url = new URL(input);
				const hashParams = new URLSearchParams(url.hash.slice(1));
				const key = hashParams.get('key');
				if (key) {
					return key;
				}
				throw new Error('Ingen gyldig nøkkel funnet i URLen');
			} catch (error) {
				throw new Error('Ugyldig URL-format');
			}
		}

		// If it's just a key, validate its format
		// Base64 validation regex
		const base64Regex = /^[A-Za-z0-9+/=_-]+$/;
		if (!base64Regex.test(input)) {
			throw new Error('Ugyldig nøkkelformat');
		}

		return input;
	}

	async function getMetadata() {
        try {
            const fileId = $page.params.fileId;
            const response = await getFileMetadata(fileId);
            const metadataResponse = await fetchMetadata(fileId, encryptionKey);

            metadata = metadataResponse;
            // Convert number to string for FileInfo component
            fileSize = response.size?.toString();
        } catch (error) {
            console.error('Metadata error:', error);
            metadata = { error: (error as Error).message };
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

        try {
            keyError = null;
            const extractedKey = validateAndExtractKey(manualKeyInput.trim());

            if (extractedKey) {
                setEncryptionKey(extractedKey);
                await getMetadata();

                if (!downloadError && !metadata?.error) {
                    await initiateDownload();
                }
            }
        } catch (error) {
            keyError = (error as Error).message;
            console.error('Key validation error:', error);
        }
    }

	async function initiateDownload() {
    if (!encryptionKey || isDownloading) return;
    isDownloading = true;
    downloadError = null;

    try {
        const fileId = $page.params.fileId;

        const { decrypted, metadata: fileMetadata } = await downloadAndDecryptFile(
            fileId,
            encryptionKey,
            async (progress, message) => {
                downloadProgress = progress;
                downloadMessage = message;
            }
        );

        if (!decrypted || decrypted.length === 0) {
            throw new Error('Kunne ikke dekryptere filen - filen er nå slettet fra serveren');
        }

        const blob = new Blob([decrypted], {
            type: fileMetadata.contentType || 'application/octet-stream'
        });

        if (blob.size === 0) {
            throw new Error('Kunne ikke dekryptere filen - filen er nå slettet fra serveren');
        }

        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = fileMetadata.filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        window.URL.revokeObjectURL(url);

        isDownloadComplete = true;

        // Clear sensitive data and reset URL to domain root
        encryptionKey = '';
        manualKeyInput = '';

        if (browser) {
            // Clean the URL without redirecting
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
            await initWasm();
            const urlParams = new URLSearchParams(window.location.hash.slice(1));
            const key = urlParams.get('key');
            if (key) {
                try {
                    const validatedKey = validateAndExtractKey(key);
                    if (validatedKey) {
                        setEncryptionKey(validatedKey);
                        await getMetadata();
                    }
                } catch (error) {
                    keyError = (error as Error).message;
                }
            }
        } catch (error) {
            console.error('Failed to initialize:', error);
            downloadError = 'Failed to initialize the application';
        }
    });;

		$: canDownload = !!(
        metadata &&
        !metadata.error &&
        !isDownloading &&
        !isDownloadComplete &&
        encryptionKey
    );
</script>

<div class="container">
	<div class="download-container">
			<h1>Last ned fil</h1>

			{#if downloadError}
					<ErrorMessage message={downloadError} />
			{:else if metadata?.error}
					<ErrorMessage message={metadata.error} />
			{:else}
					{#if metadata?.filename}
							<FileInfo
									fileName={metadata.filename}
									fileSize={fileSize}
							/>
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
				<SuccessMessage
					message="Filen er lastet ned og sikkert slettet fra serveren vår. Takk for at du bruker vår sikre fildelingstjeneste!"
				/>
			{/if}
		{/if}

		{#if !encryptionKey && !isDownloadComplete}
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
		background-color: var(--light-gray);
		border-radius: var(--border-radius);
		padding: 2rem;
	}

	h1 {
		font-size: 2.5rem;
		font-weight: 500;
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
</style>
