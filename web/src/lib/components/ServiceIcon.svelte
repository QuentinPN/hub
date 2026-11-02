<script lang="ts">
	import Monogram from './Monogram.svelte';

	// Icônes selfh.st servies par l'API : embarquées dans le binaire, ou téléchargées une fois
	// par le serveur puis gardées en base. Sans icône (null, ou 404), le monogramme coloré
	// prend le relais.
	let {
		icon,
		monogram,
		color,
		size = 44
	}: { icon: string | null; monogram: string; color: string; size?: number } = $props();

	let failed = $state(false);
</script>

{#if icon && !failed}
	<span
		class="svc-icon"
		style:width="{size}px"
		style:height="{size}px"
		style:border-radius="{Math.round(size * 0.26)}px"
	>
		<img
			src="/api/v1/icons/{encodeURIComponent(icon)}"
			alt=""
			draggable="false"
			onerror={() => (failed = true)}
		/>
	</span>
{:else}
	<Monogram text={monogram} {color} {size} />
{/if}

<style>
	.svc-icon {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--border-1);
		background: var(--surface-2);
	}
	img {
		width: 68%;
		height: 68%;
		object-fit: contain;
	}
</style>
