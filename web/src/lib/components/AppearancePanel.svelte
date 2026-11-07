<script lang="ts">
	import { onMount } from 'svelte';
	import {
		appearance,
		motionReducedByDevice,
		setAppearance,
		transparencyReducedByDevice
	} from '#lib/appearance.svelte.ts';
	import type { Density } from '#lib/appearance.ts';

	const densities: { value: Density; label: string }[] = [
		{ value: 'comfortable', label: 'Confortable' },
		{ value: 'compact', label: 'Compacte' }
	];

	// Lu après le montage : l'appareil peut le demander déjà, la case est alors cochée d'office.
	let deviceTransparency = $state(false);
	let deviceMotion = $state(false);
	onMount(() => {
		deviceTransparency = transparencyReducedByDevice();
		deviceMotion = motionReducedByDevice();
	});
</script>

<div class="head">
	<h2 id="appearance-title">Apparence</h2>
	<p class="muted">Gardée pour ce navigateur ; chaque appareil a la sienne.</p>
</div>

<div class="rows">
	<div class="row">
		<span class="text">
			<strong id="density-label">Densité</strong>
			<span class="muted" id="density-hint"
				>Compacte : plus de tuiles par rangée, pour un petit écran.</span
			>
		</span>
		<span
			class="segmented"
			role="radiogroup"
			aria-labelledby="density-label"
			aria-describedby="density-hint"
		>
			{#each densities as d (d.value)}
				<label class:on={appearance.density === d.value}>
					<input
						type="radio"
						name="density"
						value={d.value}
						checked={appearance.density === d.value}
						onchange={() => setAppearance({ density: d.value })}
					/>
					{d.label}
				</label>
			{/each}
		</span>
	</div>

	<label class="row">
		<span class="text">
			<strong>Moins de transparence</strong>
			<span class="muted">
				{deviceTransparency
					? 'Déjà demandé par l’appareil : fonds pleins partout.'
					: 'Fonds pleins au lieu du verre dépoli. Suit déjà le réglage de l’appareil.'}
			</span>
		</span>
		<input
			type="checkbox"
			checked={appearance.lessTransparency || deviceTransparency}
			disabled={deviceTransparency}
			onchange={(e) => setAppearance({ lessTransparency: e.currentTarget.checked })}
		/>
	</label>

	<label class="row">
		<span class="text">
			<strong>Réduire les animations</strong>
			<span class="muted">
				{deviceMotion
					? 'Déjà demandé par l’appareil : rien ne bouge dans Hub.'
					: 'Suit déjà le réglage de l’appareil ; ceci le force pour Hub.'}
			</span>
		</span>
		<input
			type="checkbox"
			checked={appearance.reduceMotion || deviceMotion}
			disabled={deviceMotion}
			onchange={(e) => setAppearance({ reduceMotion: e.currentTarget.checked })}
		/>
	</label>
</div>

<style>
	.head {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.head p {
		margin: 0;
	}
	.rows {
		display: flex;
		flex-direction: column;
		margin-top: 12px;
	}
	.row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		min-height: 60px;
		padding: 8px 0;
		border-top: 1px solid var(--border-1);
	}
	.row:first-child {
		border-top: 0;
	}
	.text {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.text strong {
		font-weight: 600;
	}
	.text .muted {
		font-size: 14px;
	}
	.segmented {
		display: flex;
		flex: none;
		gap: 4px;
		padding: 4px;
		border-radius: var(--r-pill);
		background: var(--surface-2);
	}
	.segmented label {
		position: relative;
		display: inline-flex;
		align-items: center;
		min-height: 40px;
		padding: 0 16px;
		border-radius: var(--r-pill);
		font-weight: 500;
		cursor: pointer;
		transition: background var(--dur) var(--ease);
	}
	.segmented label.on {
		background: var(--fill-selected);
		color: var(--text-inverse);
		font-weight: 600;
	}
	/* Le bouton radio reste dans l'arbre d'accessibilité ; l'anneau passe sur la pilule. */
	.segmented input {
		position: absolute;
		width: 1px;
		height: 1px;
		margin: 0;
		opacity: 0;
		outline: none;
	}
	.segmented label:has(:focus-visible) {
		outline: 2px solid var(--text);
		outline-offset: 2px;
	}
	input[type='checkbox'] {
		flex: none;
		width: 22px;
		height: 22px;
		margin: 0;
		accent-color: var(--status-online);
		cursor: pointer;
	}
	input[type='checkbox']:disabled {
		cursor: default;
	}
	@media (max-width: 720px) {
		.row {
			flex-wrap: wrap;
		}
	}
</style>
