<script lang="ts">
	import Footer from '$lib/components/Footer.svelte';
    import '../app.css';
</script>

<div class="layout">
    <main>
        <slot />
    </main>
    
    <Footer />
</div>


