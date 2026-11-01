<script lang="ts">
	import type { Snippet } from 'svelte';

	let { title, id, children }: { title: string; id: string; children: Snippet } = $props();
</script>

<section aria-labelledby={id}>
	<h2 {id}>{title}</h2>
	<div class="shelf">
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
