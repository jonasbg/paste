<script lang="ts">
  export let message: string;
</script>

<div class="success-message">
  <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
  >
      <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
      <polyline points="22 4 12 14.01 9 11.01" />
  </svg>
  <p>{message}</p>
</div>

<style>
  .success-message {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 16px;
      background-color: var(--success-bg);
      border: 1px solid var(--success-border);
      border-radius: 8px;
      margin-top: 16px;
  }

  .success-message svg {
      flex-shrink: 0;
      width: 24px;
      height: 24px;
      color: var(--success-icon);
  }

  .success-message p {
      margin: 0;
      color: var(--success-text);
      font-weight: 500;
      line-height: 1.5;
  }
</style>