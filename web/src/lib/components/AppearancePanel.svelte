<script lang="ts">
	import { onMount } from 'svelte';
	import {
		appearance,
		motionReducedByDevice,
		setAppearance,
		transparencyReducedByDevice
	} from '#lib/appearance.svelte.ts';
	import type { Density, Theme } from '#lib/appearance.ts';

	const themes: { value: Theme; label: string }[] = [
		{ value: 'dark', label: 'Sombre' },
		{ value: 'light', label: 'Clair' },
		{ value: 'system', label: 'Comme l’appareil' }
	];
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

<fieldset class="themes">
	<legend>Thème</legend>
	{#each themes as t (t.value)}
		<label class="theme" class:on={appearance.theme === t.value}>
			<!-- Aperçu dessiné avec les couleurs du thème, pas celles de la page. -->
			<span class="preview {t.value}" aria-hidden="true">
				{#if t.value === 'system'}
					<span class="half dark"></span><span class="half light"></span>
				{:else}
					<span class="bar"></span>
					<span class="tiles"><span></span><span></span></span>
				{/if}
			</span>
			<span class="choice">
				<input
					type="radio"
					name="theme"
					value={t.value}
					checked={appearance.theme === t.value}
					onchange={() => setAppearance({ theme: t.value })}
				/>
				{t.label}
			</span>
		</label>
	{/each}
</fieldset>

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
	/* Les aperçus montrent chaque thème tel qu'il est, quel que soit celui de la page. */
	.themes {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 14px;
		margin: 18px 0 0;
		padding: 0;
		border: 0;
	}
	legend {
		margin-bottom: 10px;
		padding: 0;
		font-weight: 500;
	}
	.theme {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 12px;
		border: 2px solid var(--border-2);
		border-radius: 18px;
		background: var(--surface-inset);
		cursor: pointer;
	}
	.theme.on {
		border-color: var(--text);
	}
	.theme:has(:focus-visible) {
		outline: 2px solid var(--text);
		outline-offset: 3px;
	}
	.preview {
		display: flex;
		flex-direction: column;
		gap: 8px;
		height: 110px;
		box-sizing: border-box;
		padding: 12px;
		overflow: hidden;
		border: 1px solid var(--border-1);
		border-radius: 12px;
	}
	.preview.dark,
	.half.dark {
		--p-bg: #07080a;
		--p-ink: #f5f5f7;
		--p-tile: rgba(255, 255, 255, 0.1);
	}
	.preview.light,
	.half.light {
		--p-bg: #ffffff;
		--p-ink: #1d1d1f;
		--p-tile: rgba(0, 0, 0, 0.08);
	}
	.preview.dark,
	.preview.light {
		background: var(--p-bg);
	}
	.preview.system {
		flex-direction: row;
		gap: 0;
		padding: 0;
	}
	.half {
		flex: 1;
		background: var(--p-bg);
	}
	.bar {
		width: 40%;
		height: 10px;
		border-radius: 6px;
		background: var(--p-ink);
	}
	.tiles {
		display: flex;
		flex: 1;
		gap: 8px;
	}
	.tiles span {
		flex: 1;
		border-radius: 8px;
		background: var(--p-tile);
	}
	.choice {
		display: flex;
		align-items: center;
		gap: 10px;
		font-weight: 600;
	}
	.choice input {
		width: 20px;
		height: 20px;
		margin: 0;
		accent-color: var(--text);
		outline: none;
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
		background: var(--track-soft);
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
		.themes {
			grid-template-columns: minmax(0, 1fr);
			gap: 10px;
		}
		.theme {
			flex-direction: row-reverse;
			justify-content: space-between;
			align-items: center;
		}
		.preview {
			flex: none;
			width: 64px;
			height: 44px;
			padding: 6px;
			gap: 4px;
		}
		.bar {
			height: 6px;
		}
		.tiles {
			gap: 4px;
		}
		.tiles span {
			border-radius: 4px;
		}
		.row {
			flex-wrap: wrap;
		}
	}
</style>
