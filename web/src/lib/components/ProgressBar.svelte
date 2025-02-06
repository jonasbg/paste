<script lang="ts">
  export let progress: number;
  export let message: string;
</script>

<div class="download-progress">
  <div class="progress-title">{message}</div>
  <div class="download-progress-bar">
      <div
          class="download-progress-fill"
          style="width: {progress}%"
      ></div>
  </div>
  <div class="download-progress-text">
      {Math.round(progress)}%
  </div>
</div>

<style>
  .download-progress {
      margin-top: 1rem;
      background-color: var(--light-gray);
      border-radius: var(--border-radius);
      padding: 1rem;
  }

  .progress-title {
      font-size: 1.25rem;
      margin-bottom: 1rem;
      font-weight: 500;
  }

  .download-progress-bar {
      width: 100%;
      height: 8px;
      background-color: #e0e0e0;
      border-radius: 4px;
      overflow: hidden;
      margin-top: 0.5rem;
  }

  .download-progress-fill {
      width: 0%;
      height: 100%;
      background-color: var(--primary-green);
      transition: width 0.3s ease-in-out;
  }

  .download-progress-text {
      margin-top: 0.5rem;
      font-size: 0.875rem;
      color: #666;
  }
</style>
