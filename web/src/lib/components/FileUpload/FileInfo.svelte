<script lang="ts">
    export let fileName: string = '';
    export let fileSize: string = '';
    export let isVisible: boolean = false;
</script>

<div class="file-info" style="display: {isVisible ? 'block' : 'none'}">
    <div class="progress-title">Filinformasjon</div>
    <div class="file-info-item">Filnavn: {fileName}</div>
    <div class="file-info-item">Størrelse: {fileSize}</div>
</div>

<style>
    .file-info {
        margin: 1rem 0;
        padding: 1rem;
        background-color: white;
        border-radius: var(--border-radius);
    }

    .file-info-item {
        margin: 0.5rem 0;
        color: #666;
    }

    .progress-title {
        font-size: 1.25rem;
        margin-bottom: 1rem;
        font-weight: 500;
    }
</style>