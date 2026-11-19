<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { UpcomingItem } from '#lib/api/types.ts';
	import PosterCard from './PosterCard.svelte';

	// L'affiche reste celle des autres rangées ; ce qui s'y fait se pose dessous.
	let { item, children }: { item: UpcomingItem & { image?: string }; children: Snippet } = $props();
</script>

<div class="cell">
	<PosterCard {item} />
	<div class="actions">{@render children()}</div>
</div>

<style>
	.cell {
		display: grid;
		flex: 0 0 var(--poster-w);
		grid-template-columns: minmax(0, 1fr);
		align-content: start;
		gap: 10px;
		width: var(--poster-w);
	}
	.actions {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}
</style>
