<script lang="ts">
	import type { UpcomingItem } from '#lib/api/types.ts';

	let { item }: { item: UpcomingItem & { image?: string } } = $props();

	// Sans image, ou si le service n'en a pas : l'aplat tonal reste (tone vient de l'API).
	let failed = $state(false);
</script>

<article class="poster" style:background="linear-gradient(160deg, {item.tone}, #0d0f12)">
	{#if item.image && !failed}
		<img src={item.image} alt="" loading="lazy" onerror={() => (failed = true)} />
		<span class="shade"></span>
	{/if}
	<span class="chip" title={item.when}>{item.when}</span>
	<div class="text">
		<div class="title">{item.title}</div>
		<div class="detail muted">{item.detail}</div>
	</div>
</article>

<style>
	.poster {
		position: relative;
		display: flex;
		flex: 0 0 168px;
		flex-direction: column;
		justify-content: space-between;
		height: 252px;
		box-sizing: border-box;
		overflow: hidden;
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
	img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	/* Le titre reste lisible, quelle que soit l'affiche. */
	.shade {
		position: absolute;
		inset: 0;
		background: linear-gradient(180deg, rgba(0, 0, 0, 0) 45%, rgba(0, 0, 0, 0.88) 100%);
	}
	.chip,
	.text {
		position: relative;
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
		background: rgba(0, 0, 0, 0.6);
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
	.shade + .chip + .text .detail {
		color: #d1d1d6;
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
