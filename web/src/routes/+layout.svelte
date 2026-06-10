<script lang="ts">
	import Footer from '$lib/components/Footer.svelte';
    import '../app.css';
    interface Props {
        children?: import('svelte').Snippet;
    }

    let { children }: Props = $props();
</script>

<div class="layout">
    <main>
        {@render children?.()}
    </main>
    
    <Footer />
</div>


