<script lang="ts">
	import Footer from '$lib/components/Footer.svelte';
	import Home from './pages/Home.svelte';
	import Download from './pages/Download.svelte';

	// Path-based routing for the static SPA: "/" → upload, "/{fileId}" → download.
	// The Go server (and Vite's preview) serve index.html for any path, so we route
	// on the pathname here. Evaluated once at load — the app moves between the two
	// views via full page loads (a shared link or the reset link), never in-page.
	const path = typeof window !== 'undefined' ? window.location.pathname : '/';
	const fileId = decodeURIComponent(path.replace(/^\/+/, '').split('/')[0] ?? '');
</script>

<div class="layout">
	<main>
		{#if fileId}
			<Download {fileId} />
		{:else}
			<Home />
		{/if}
	</main>

	<Footer />
</div>
