<script lang="ts">
  export let url: string = '';
  export let isVisible: boolean = false;

  function copyUrl() {
      const urlField = document.querySelector('input') as HTMLInputElement;
      urlField.select();
      document.execCommand('copy');
      alert('Lenke kopiert til utklippstavlen!');
  }
</script>

<div class="url-container" style="display: {isVisible ? 'block' : 'none'}">
  <input type="text" class="url-field" value={url} readonly />
  <button class="button" on:click={copyUrl}>Kopier lenke</button>
</div>

<style>
  .url-container {
      margin-top: 1rem;
  }

  .url-field {
      width: 100%;
      padding: 0.75rem;
      border: 1px solid #e0e0e0;
      border-radius: var(--border-radius);
      margin-bottom: 0.5rem;
      font-family: inherit;
  }
</style>