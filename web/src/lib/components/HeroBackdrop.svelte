<script lang="ts">
	import { fade } from 'svelte/transition';
	import { motionReduced } from '#lib/appearance.svelte.ts';

	// Le fond de ce qui joue. Il n'apparaît qu'une fois chargé ; sans image, ou si le service
	// n'en a pas, les couleurs du bandeau restent. Une image par src : le parent la recrée.
	let { src }: { src: string } = $props();

	let loaded = $state(false);
	let failed = $state(false);
</script>

{#if !failed}
	<div class="backdrop" class:shown={loaded} out:fade={{ duration: motionReduced() ? 0 : 400 }}>
		<img
			{src}
			alt=""
			decoding="async"
			onload={() => (loaded = true)}
			onerror={() => (failed = true)}
		/>
		<span class="scrim"></span>
	</div>
{/if}

<style>
	.backdrop {
		position: absolute;
		inset: 0;
		opacity: 0;
		transition: opacity 600ms var(--ease);
	}
	.shown {
		opacity: 1;
	}
	img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		object-position: center 25%;
	}
	/* Le texte se pose à gauche et en bas : le fond s'y efface vers la page, dans les deux
	   thèmes, et le bas rejoint les rangées sans couture. */
	.scrim {
		position: absolute;
		inset: 0;
		background:
			linear-gradient(
				90deg,
				var(--bg) 0%,
				color-mix(in srgb, var(--bg) 78%, transparent) 30%,
				color-mix(in srgb, var(--bg) 20%, transparent) 65%,
				transparent 100%
			),
			linear-gradient(
				180deg,
				color-mix(in srgb, var(--bg) 45%, transparent) 0%,
				transparent 30%,
				transparent 55%,
				var(--bg) 100%
			);
	}
	@media (prefers-reduced-motion: reduce) {
		.backdrop {
			transition: none;
		}
	}
	:global(:root[data-motion='reduce']) .backdrop {
		transition: none;
	}
	@media (max-width: 640px) {
		.scrim {
			background: linear-gradient(
				180deg,
				color-mix(in srgb, var(--bg) 45%, transparent) 0%,
				color-mix(in srgb, var(--bg) 50%, transparent) 35%,
				var(--bg) 85%
			);
		}
	}
</style>
