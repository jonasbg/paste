<script lang="ts">
    import { fade, fly } from 'svelte/transition';
    export let fileName: string = '';
    export let fileSize: string = '';
    export let isVisible: boolean = false;
    export let onRemove: () => void;
</script>

{#if isVisible}
<div class="file-info" 
     in:fly={{ y: 20, duration: 300 }} 
     out:fade={{ duration: 200 }}>
    <button class="remove-button" on:click={onRemove} aria-label="Remove file">
        <svg 
            width="20" 
            height="20" 
            viewBox="0 0 24 24" 
            fill="none" 
            stroke="currentColor" 
            stroke-width="2"
            stroke-linecap="round" 
            stroke-linejoin="round"
        >
            <circle cx="12" cy="12" r="10" />
            <line x1="15" y1="9" x2="9" y2="15" />
            <line x1="9" y1="9" x2="15" y2="15" />
        </svg>
    </button>
    <div class="file-details">
        <div class="file-icon">
            <svg 
                width="24" 
                height="24" 
                viewBox="0 0 24 24" 
                fill="none" 
                stroke="currentColor" 
                stroke-width="2"
                stroke-linecap="round" 
                stroke-linejoin="round"
            >
                <path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z" />
                <polyline points="13 2 13 9 20 9" />
            </svg>
        </div>
        <div class="file-info-content">
            <div class="filename">{fileName}</div>
            <div class="filesize">{fileSize}</div>
        </div>
    </div>
</div>
{/if}

<style>
    .file-info {
        background-color: white;
        border-radius: 8px;
        padding: 1rem;
        box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
        position: relative;
        border: 1px solid #eee;
        transition: all 0.2s ease;
        margin-bottom: 2rem;
    }

    .file-info:hover {
        box-shadow: 0 4px 8px rgba(0, 0, 0, 0.1);
    }

    .remove-button {
        position: absolute;
        top: -10px;
        right: -10px;
        background: #000;
        border: none;
        border-radius: 50%;
        width: 24px;
        height: 24px;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
        padding: 0;
        color: white;
        transition: transform 0.2s ease;
    }

    .remove-button:hover {
        transform: scale(1.1);
    }

    .file-details {
        display: flex;
        align-items: center;
        gap: 1rem;
    }

    .file-icon {
        color: #666;
    }

    .file-info-content {
        flex: 1;
    }

    .filename {
        font-weight: 500;
        margin-bottom: 0.25rem;
        word-break: break-all;
    }

    .filesize {
        color: #666;
        font-size: 0.875rem;
    }
</style>