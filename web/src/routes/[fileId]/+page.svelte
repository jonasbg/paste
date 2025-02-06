<script lang="ts">
    import { onMount } from 'svelte';
    import { browser } from '$app/environment';
    import { page } from '$app/stores';
    import { initWasm } from '$lib/utils/wasm-loader';
    import { downloadAndDecryptFile, fetchMetadata } from '$lib/services/fileService';
    import ErrorMessage from '$lib/components/ErrorMessage.svelte';
    import SuccessMessage from '$lib/components/SuccessMessage.svelte';
    import ProgressBar from '$lib/components/Shared/ProgressBar.svelte';
    import FileInfo from '$lib/components/FileInfo.svelte';

    let encryptionKey: string = '';
    let metadata: any = null;
    let downloadProgress = 0;
    let downloadMessage = '';
    let isDownloading = false;
    let downloadError: string | null = null;
    let isDownloadComplete = false;

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

        const blob = new Blob([decrypted], {
            type: fileMetadata.contentType || 'application/octet-stream'
        });
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = fileMetadata.filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        window.URL.revokeObjectURL(url);

        isDownloadComplete = true;
    } catch (error) {
        console.error('Download error:', error);
        downloadError = (error as Error).message;
    } finally {
        isDownloading = false;
    }
}

    async function getFileMetadata() {
        try {
            const fileId = $page.params.fileId;
            metadata = await fetchMetadata(fileId, encryptionKey);
        } catch (error) {
            console.error('Metadata error:', error);
            metadata = { error: (error as Error).message };
        }
    }

    async function handleKeyInput(event: Event) {
        const input = event.target as HTMLInputElement;
        const hash = new URL(input.value).hash;
        if (hash) {
            const params = new URLSearchParams(hash.slice(1));
            const key = params.get('key');
            if (key) {
                encryptionKey = key;
                window.location.hash = `key=${key}`;
                await getFileMetadata();
            }
        }
    }

    onMount(async () => {
        if (!browser) return;

        try {
            await initWasm();
            const urlParams = new URLSearchParams(window.location.hash.slice(1));
            encryptionKey = urlParams.get('key') || '';
            await getFileMetadata();
        } catch (error) {
            console.error('Failed to initialize:', error);
            downloadError = 'Failed to initialize the application';
        }
    });

    $: canDownload = !!(metadata && !metadata.error && !isDownloading && !isDownloadComplete && encryptionKey);
</script>

<div class="container">
    <div class="download-container">
        <h1>Last ned fil</h1>

        {#if downloadError}
            <ErrorMessage message={downloadError} />
        {:else if !metadata}
            <FileInfo filename="Laster filinformasjon..." />
        {:else if metadata.error}
            <ErrorMessage message={metadata.error} />
        {:else}
            <FileInfo filename={metadata.filename} />

            {#if !isDownloadComplete}
            {#if !isDownloading}
                <button class="button" on:click={initiateDownload} disabled={!canDownload}>
                    {isDownloading ? 'Laster ned...' : 'Last ned'}
                </button>
                {/if}
            {/if}

            {#if isDownloading || isDownloadComplete}
                <ProgressBar progress={downloadProgress} message={downloadMessage} isVisible={isDownloading} />
            {/if}

            {#if isDownloadComplete}
                <SuccessMessage
                    message="Filen er lastet ned og sikkert slettet fra serveren vår. Takk for at du bruker vår sikre fildelingstjeneste!"
                />
            {/if}
        {/if}

        {#if !encryptionKey}
            <div class="key-prompt">
                <h2>Dekrypteringsnøkkel kreves</h2>
                <p>
                    Du trenger en dekrypteringsnøkkel for å få tilgang til denne filen.
                    Vennligst lim inn hele lenken du har mottatt, så vil nøkkelen automatisk bli hentet ut.
                </p>
                <input
                    type="text"
                    class="key-input"
                    placeholder="Lim inn hele fildelingslenken her"
                    on:paste={handleKeyInput}
                    on:input={handleKeyInput}
                />
            </div>
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
        margin-bottom: 1.5rem;
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
        border: 1px solid #E0E0E0;
        border-radius: var(--border-radius);
        margin-bottom: 1rem;
        font-family: inherit;
    }

    .key-input:focus {
        outline: none;
        border-color: var(--primary-green);
        box-shadow: 0 0 0 2px rgba(64, 184, 123, 0.2);
    }
</style>