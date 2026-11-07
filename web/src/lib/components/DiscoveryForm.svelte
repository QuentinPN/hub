<script lang="ts">
	import { untrack } from 'svelte';
	import { updateDiscovery } from '#lib/api/client.ts';
	import type { Discovery, RemovalPolicy } from '#lib/api/types.ts';

	let { initial, editable }: { initial: Discovery; editable: boolean } = $props();

	// Copie locale modifiable ; `saved` est la dernière valeur confirmée par le serveur.
	let saved = $state(untrack(() => ({ ...initial })));
	let form = $state(untrack(() => ({ ...initial })));
	let saving = $state(false);
	let message = $state<{ ok: boolean; text: string } | null>(null);

	const dirty = $derived(JSON.stringify(form) !== JSON.stringify(saved));

	const rescanChoices = [5, 15, 60];
	const confirmChoices = [70, 80, 90];
	const removalPolicies: RemovalPolicy[] = ['absent_then_archive', 'absent_only', 'archive_now'];
	const removalLabel = (policy: RemovalPolicy, days: number) =>
		({
			absent_then_archive: `Marquer « absent », archiver après ${days} jours`,
			absent_only: 'Marquer « absent » sans archiver',
			archive_now: 'Archiver tout de suite'
		})[policy];
	const minutesLabel = (m: number) => (m === 60 ? 'Toutes les heures' : `Toutes les ${m} min`);
	// Une valeur venue du serveur hors des choix proposés reste affichée telle quelle.
	const withCurrent = (choices: number[], current: number) =>
		[...new Set([...choices, current])].sort((a, b) => a - b);

	async function save(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		message = null;
		try {
			const settings = await updateDiscovery(fetch, form);
			saved = { ...settings.discovery };
			form = { ...settings.discovery };
			message = { ok: true, text: 'Réglages enregistrés.' };
		} catch (err) {
			message = {
				ok: false,
				text: `Échec de l'enregistrement : ${err instanceof Error ? err.message : String(err)}`
			};
		} finally {
			saving = false;
		}
	}
</script>

<form onsubmit={save}>
	<div class="set">
		<span class="lbl" id="events-label"
			>Suivre les événements Docker<small>Un nouveau conteneur apparaît en quelques secondes.</small
			></span
		>
		<button
			type="button"
			class="switch"
			role="switch"
			aria-checked={form.followEvents}
			aria-labelledby="events-label"
			disabled={!editable}
			onclick={() => (form.followEvents = !form.followEvents)}><span></span></button
		>
	</div>
	<div class="set">
		<label for="rescan"
			>Rescan de réconciliation<small>Rattrape ce que les événements auraient manqué.</small></label
		>
		<select id="rescan" disabled={!editable} bind:value={form.rescanMinutes}>
			{#each withCurrent(rescanChoices, saved.rescanMinutes) as m (m)}
				<option value={m}>{minutesLabel(m)}</option>
			{/each}
		</select>
	</div>
	<div class="set">
		<label for="removal"
			>Quand un service disparaît<small>Ses réglages, favoris et URL sont toujours conservés.</small
			></label
		>
		<select id="removal" disabled={!editable} bind:value={form.onRemoval}>
			{#each removalPolicies as p (p)}
				<option value={p}>{removalLabel(p, form.archiveAfterDays)}</option>
			{/each}
		</select>
	</div>
	<div class="set">
		<label for="confirm"
			>Demander confirmation si la confiance est sous<small
				>Sous ce seuil, la carte reste « à confirmer ».</small
			></label
		>
		<select id="confirm" disabled={!editable} bind:value={form.confirmBelowPercent}>
			{#each withCurrent(confirmChoices, saved.confirmBelowPercent) as p (p)}
				<option value={p}>{p} %</option>
			{/each}
		</select>
	</div>
	<div class="set">
		<span class="lbl" id="infer-label"
			>Déduire une URL d'un port publié<small
				>Désactivé par sécurité : seuls le label <span class="mono">homelab.url</span> ou ta saisie font
				foi.</small
			></span
		>
		<button
			type="button"
			class="switch"
			role="switch"
			aria-checked="false"
			aria-labelledby="infer-label"
			disabled><span></span></button
		>
	</div>

	{#if editable}
		<div class="actions">
			<p class="status" class:error={message && !message.ok} role="status">
				{message?.text ?? ''}
			</p>
			<button
				type="button"
				class="btn ghost"
				disabled={!dirty || saving}
				onclick={() => (form = { ...saved })}>Annuler</button
			>
			<button type="submit" class="btn primary" disabled={!dirty || saving}
				>{saving ? 'Enregistrement…' : 'Enregistrer'}</button
			>
		</div>
	{/if}
</form>

<style>
	.set {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 24px;
		padding: 16px 0;
		border-top: 1px solid var(--border-1);
	}
	.set:first-child {
		border-top: 0;
	}
	.set label,
	.lbl {
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 15px;
	}
	small {
		color: var(--text-2);
		font-size: var(--fs-small);
	}
	select {
		min-height: 44px;
		max-width: 50%;
		padding: 0 12px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		color: var(--text);
		font: inherit;
		font-size: 14px;
	}
	select:disabled {
		opacity: 0.75;
	}
	.switch {
		position: relative;
		flex: none;
		width: 51px;
		height: 31px;
		border: 0;
		border-radius: 99px;
		background: var(--status-online);
		cursor: pointer;
	}
	.switch span {
		position: absolute;
		top: 2px;
		right: 2px;
		width: 27px;
		height: 27px;
		border-radius: 99px;
		background: var(--on-color);
		box-shadow: var(--shadow-knob);
		transition:
			left var(--dur) var(--ease),
			right var(--dur) var(--ease);
	}
	.switch[aria-checked='false'] {
		background: var(--track-strong);
	}
	.switch[aria-checked='false'] span {
		right: auto;
		left: 2px;
	}
	.switch:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: 10px;
		padding-top: 16px;
		border-top: 1px solid var(--border-1);
	}
	.status {
		flex: 1;
		min-width: 200px;
		margin: 0;
		color: var(--text-ok);
		font-size: 14px;
	}
	.status.error {
		color: var(--text-danger);
	}
	.btn {
		min-height: 44px;
		padding: 0 18px;
		border: 0;
		border-radius: 12px;
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.primary {
		background: var(--text);
		color: var(--text-inverse);
	}
	.ghost {
		background: transparent;
		color: var(--text-2);
	}
	@media (max-width: 720px) {
		.set {
			flex-wrap: wrap;
			gap: 10px;
		}
		select {
			max-width: 100%;
		}
	}
</style>
