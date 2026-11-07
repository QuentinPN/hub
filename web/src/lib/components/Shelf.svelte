<script lang="ts">
	import type { Snippet } from 'svelte';

	// focusable : rangée sans lien (affiches) ; le clavier doit pouvoir la faire défiler.
	let {
		title,
		id,
		focusable = false,
		children
	}: { title: string; id: string; focusable?: boolean; children: Snippet } = $props();

	let shelf: HTMLDivElement | undefined = $state();
	let canPrev = $state(false);
	let canNext = $state(false);

	// Où mène le défilement en cours : un clic pendant l'animation part de là, pas de la
	// position du moment, sinon chaque clic rapproché perd du chemin.
	let target: number | null = null;

	function measure() {
		if (!shelf) return;
		// 1 px de marge : les largeurs fractionnaires laissent souvent un reste.
		canPrev = shelf.scrollLeft > 1;
		canNext = shelf.scrollLeft + shelf.clientWidth < shelf.scrollWidth - 1;
		if (target !== null && Math.abs(shelf.scrollLeft - target) <= 1) target = null;
	}

	// Une souris sans molette horizontale n'a aucun autre moyen de défiler la rangée.
	function page(direction: 1 | -1) {
		if (!shelf) return;
		const end = shelf.scrollWidth - shelf.clientWidth;
		const from = target ?? shelf.scrollLeft;
		target = Math.min(end, Math.max(0, from + direction * shelf.clientWidth * 0.85));
		// L'état des flèches suit la destination, sans attendre la fin de l'animation.
		canPrev = target > 1;
		canNext = target < end - 1;
		const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
		shelf.scrollTo({ left: target, behavior: reduce ? 'auto' : 'smooth' });
	}

	$effect(() => {
		if (!shelf) return;
		const observer = new ResizeObserver(measure);
		observer.observe(shelf);
		// Le contenu change sans que la rangée change de taille (favoris ajoutés).
		for (const child of shelf.children) observer.observe(child);
		measure();
		return () => observer.disconnect();
	});
</script>

<section aria-labelledby={id}>
	<div class="head">
		<h2 {id}>{title}</h2>
		{#if canPrev || canNext}
			<div class="nav">
				<button type="button" aria-label="Précédents" disabled={!canPrev} onclick={() => page(-1)}>
					<svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
						<path d="M15 5l-7 7 7 7" />
					</svg>
				</button>
				<button type="button" aria-label="Suivants" disabled={!canNext} onclick={() => page(1)}>
					<svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
						<path d="M9 5l7 7-7 7" />
					</svg>
				</button>
			</div>
		{/if}
	</div>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div
		class="shelf"
		class:fade-start={canPrev}
		class:fade-end={canNext}
		tabindex={focusable ? 0 : undefined}
		bind:this={shelf}
		onscroll={measure}
		onscrollend={() => {
			target = null;
			measure();
		}}
	>
		{@render children()}
	</div>
</section>

<style>
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
		letter-spacing: -0.01em;
	}
	.nav {
		display: flex;
		gap: 8px;
	}
	.nav button {
		display: grid;
		place-items: center;
		width: 34px;
		height: 34px;
		padding: 0;
		border: 1px solid var(--border-1);
		border-radius: var(--r-pill);
		background: var(--surface-1);
		color: var(--text);
		cursor: pointer;
		transition:
			background var(--dur) var(--ease),
			opacity var(--dur) var(--ease);
	}
	.nav button:hover:not(:disabled) {
		background: var(--surface-3);
	}
	.nav button:focus-visible {
		outline: 2px solid var(--border-focus);
		outline-offset: 2px;
	}
	.nav button:disabled {
		opacity: 0.35;
		cursor: default;
	}
	.nav path {
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}
	/* Défilement horizontal natif : molette, glisser au doigt et Tab fonctionnent. */
	.shelf {
		--fade-start: 0%;
		--fade-end: 100%;
		display: flex;
		gap: 28px;
		padding: 18px 6px 30px;
		margin: 0 -6px;
		overflow-x: auto;
		scroll-snap-type: x proximity;
		/* Sans elle, l'aimant colle la 1re carte au bord et décale la rangée de 6 px. */
		scroll-padding-inline: 6px;
		scrollbar-width: none;
		mask-image: linear-gradient(
			90deg,
			transparent,
			#000 var(--fade-start),
			#000 var(--fade-end),
			transparent
		);
	}
	.shelf.fade-start {
		--fade-start: 6%;
	}
	.shelf.fade-end {
		--fade-end: 90%;
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
	/* Au doigt, glisser suffit ; les flèches prendraient la place du titre. */
	@media (hover: none) {
		.nav {
			display: none;
		}
	}
</style>
