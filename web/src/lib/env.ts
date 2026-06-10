// Replacement for SvelteKit's `$app/environment` in this plain Svelte + Vite SPA.
// There is no SSR, so `browser` is effectively always true at runtime; the guard is
// kept so existing `if (browser)` checks (and any non-DOM execution context) stay safe.
export const browser = typeof window !== 'undefined';
