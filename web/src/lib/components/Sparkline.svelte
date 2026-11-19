<script lang="ts">
	import { sparkline, sparklineSummary } from '#lib/request.ts';

	let { byMonth }: { byMonth: { month: string; count: number }[] } = $props();

	const WIDTH = 240;
	const HEIGHT = 56;
	const shape = $derived(
		sparkline(
			byMonth.map((m) => m.count),
			WIDTH,
			HEIGHT
		)
	);
	const summary = $derived(sparklineSummary(byMonth));
</script>

<svg viewBox="0 0 {WIDTH} {HEIGHT}" role="img" aria-label="Demandes par mois. {summary}">
	{#if shape.line}
		<path class="area" d={shape.area} />
		<path class="line" d={shape.line} />
	{/if}
	{#if shape.end}
		<circle class="end" cx={shape.end.x} cy={shape.end.y} r="4" />
	{/if}
</svg>

<style>
	svg {
		display: block;
		width: 100%;
		height: auto;
		overflow: visible;
	}
	.area {
		fill: var(--tint-accent-bg-strong);
	}
	.line {
		fill: none;
		stroke: var(--accent-play);
		stroke-width: 2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}
	/* Le point d'arrivée, cerclé de la surface comme les repères du reste de Hub. */
	.end {
		fill: var(--accent-play);
		stroke: var(--bg);
		stroke-width: 2;
	}
</style>
