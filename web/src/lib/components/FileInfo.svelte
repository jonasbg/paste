<script lang="ts">
  export let filename: string;
</script>

<div class="file-info">
  <div class="file-info-item">Fil: {filename}</div>
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
</style>