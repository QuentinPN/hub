<script lang="ts">
	import type { UpcomingItem } from '#lib/api/types.ts';

	let { item }: { item: UpcomingItem } = $props();
</script>

<!-- Pas encore d'affiches réelles : un aplat tonal tient lieu d'image (tone vient de l'API). -->
<article class="poster" style:background="linear-gradient(160deg, {item.tone}, #0d0f12)">
	<span class="chip" title={item.when}>{item.when}</span>
	<div>
		<div class="title">{item.title}</div>
		<div class="detail muted">{item.detail}</div>
	</div>
</article>

<style>
	.poster {
		display: flex;
		flex: 0 0 168px;
		flex-direction: column;
		justify-content: space-between;
		height: 252px;
		padding: 14px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-poster);
		transition:
			transform var(--dur) var(--ease),
			box-shadow var(--dur) var(--ease);
	}
	.poster:hover {
		transform: scale(var(--focus-scale));
		border-color: var(--border-focus);
		box-shadow: var(--shadow-lift);
	}
	/* Toujours sur une ligne : un texte trop long est coupé, jamais renvoyé à la ligne. */
	.chip {
		align-self: flex-start;
		max-width: 100%;
		overflow: hidden;
		padding: 4px 9px;
		text-overflow: ellipsis;
		white-space: nowrap;
		border-radius: var(--r-pill);
		background: rgba(0, 0, 0, 0.45);
		font-size: 12px;
		font-weight: 600;
	}
	.title {
		font-size: 16px;
		font-weight: 600;
	}
	.detail {
		font-size: var(--fs-small);
	}
	@media (max-width: 720px) {
		.poster {
			flex-basis: 118px;
			height: 176px;
			padding: 10px;
		}
		.chip {
			padding: 3px 7px;
			font-size: 11px;
		}
	}
</style>
