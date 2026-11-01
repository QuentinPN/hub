<script lang="ts">
	import type { Snippet } from 'svelte';

	// focusable : rangée sans lien (affiches) ; le clavier doit pouvoir la faire défiler.
	let {
		title,
		id,
		focusable = false,
		children
	}: { title: string; id: string; focusable?: boolean; children: Snippet } = $props();
</script>

<section aria-labelledby={id}>
	<h2 {id}>{title}</h2>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div class="shelf" tabindex={focusable ? 0 : undefined}>
		{@render children()}
	</div>
</section>

<style>
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
		letter-spacing: -0.01em;
	}
	/* Défilement horizontal natif : molette, glisser au doigt et Tab fonctionnent. */
	.shelf {
		display: flex;
		gap: 28px;
		padding: 18px 6px 30px;
		margin: 0 -6px;
		overflow-x: auto;
		scroll-snap-type: x proximity;
		scrollbar-width: none;
		mask-image: linear-gradient(90deg, #000 90%, transparent);
	}
	.shelf::-webkit-scrollbar {
		display: none;
	}
	.shelf > :global(*) {
		scroll-snap-align: start;
	}
	@media (max-width: 720px) {
		.shelf {
			gap: 12px;
		}
	}
</style>
