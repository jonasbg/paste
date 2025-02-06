<script lang="ts">
  export let progress: number = 0;
  export let message: string = '';
  export let isVisible: boolean = false;
</script>

<div class="progress-container" style="display: {isVisible ? 'block' : 'none'}">
  <div class="progress-title">{message}</div>
  <div class="progress-bar">
      <div class="progress" style="width: {progress}%"></div>
  </div>
  <div class="progress-text">{Math.round(progress)}%</div>
</div>

<style>
  .progress-container {
      margin-top: 2rem;
      background-color: var(--light-gray);
      border-radius: var(--border-radius);
      padding: 2rem;
  }

  .progress-title {
      font-size: 1.25rem;
      margin-bottom: 1rem;
      font-weight: 500;
  }

  .progress-bar {
      width: 100%;
      height: 8px;
      background-color: #e0e0e0;
      border-radius: 4px;
      overflow: hidden;
  }

  .progress {
      height: 100%;
      background-color: var(--primary-green);
      transition: width 0.3s ease-in-out;
  }

  .progress-text {
      margin-top: 0.5rem;
      font-size: 0.875rem;
      color: #666;
  }
</style>