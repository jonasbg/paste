<footer>
  <div class="container">
    <a href="https://github.com/jonasbg/paste" target="_blank" rel="noopener noreferrer" class="github-link">
      <span class="pill">paste source</span>
      <div class="icon-container">
        <svg class="github-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
          <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
        </svg>
      </div>
    </a>
  </div>
</footer>

<style>
  footer {
    position: fixed;
    bottom: 20px;
    right: 20px;
    z-index: 999;
  }

  .container {
    padding: 0;
    position: relative;
    display: flex;
    justify-content: flex-end;
  }

  .github-link {
    position: relative;
    display: flex;
    align-items: center;
    text-decoration: none;
    transition: transform 0.3s cubic-bezier(0.175, 0.885, 0.32, 1.275);
  }

  .github-link:hover {
    transform: scale(1.05);
  }

  .icon-container {
    background-color: white;
    border-radius: 50%;
    width: 2.75rem;
    height: 2.75rem;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.3s ease;
    position: relative;
    z-index: 2;
    /* Tighter padding around the icon */
    padding: 0.5rem;
    box-sizing: border-box;
  }

  .github-link:hover .icon-container {
    transform: translateX(3px);
  }

  .github-icon {
    width: 1.6rem;
    height: 1.6rem;
    fill: #24292e;
    transition: transform 0.3s ease;
  }

  .github-link:hover .github-icon {
    transform: rotate(5deg);
  }

  .pill {
    padding-right: 2em !important;
    background-color: #24292e;
    color: white;
    padding: 0.6rem 1.2rem;
    border-radius: 3rem;
    margin-right: -1.4rem;
    font-size: 0.95rem;
    font-weight: 500;
    opacity: 0;
    transform: translateX(0) scale(0.5);
    transform-origin: right center;
    position: relative;
    left: 1.5rem;
    transition: all 0.4s cubic-bezier(0.175, 0.885, 0.32, 1.275);
    pointer-events: none;
    letter-spacing: 0.03em;
    white-space: nowrap;
  }

  .github-link:hover .pill {
    opacity: 1;
    transform: translateX(-2rem) scale(1);
    pointer-events: auto;
  }

  @keyframes pulse {
    0% { transform: scale(1); }
    50% { transform: scale(1.08); }
    100% { transform: scale(1); }
  }

  .icon-container {
    animation: pulse 2s ease-in-out 1;
  }
</style>