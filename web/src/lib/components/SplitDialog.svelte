<script lang="ts">
	import { ApiError, saveServiceUrl, splitService } from '#lib/api/client.ts';
	import type { ServiceContainer, ServiceDetail } from '#lib/api/types.ts';
	import { splitName } from '#lib/containers.ts';
	import ServiceIcon from './ServiceIcon.svelte';

	interface Props {
		service: ServiceDetail;
		/** Un conteneur du service Compose à mettre à part. */
		container: ServiceContainer;
		/** Les autres services Compose, qui restent au service. */
		others: string[];
		onclose: () => void;
		/** Service créé ; `urlSaved` dit si l'adresse tapée a été gardée. */
		ondone: (created: ServiceDetail, urlSaved: boolean) => void;
	}
	let { service, container, others, onclose, ondone }: Props = $props();

	const CATEGORIES = ['Médias', 'Téléchargements', 'Administration', 'Mes applications', 'Autres'];
	const composeService = $derived(container.composeService);

	// Le formulaire part des valeurs à l'ouverture.
	const initial = () => ({ service, container });
	let name = $state(splitName(initial().service.name, initial().container.composeService));
	let category = $state(initial().service.category);
	let address = $state('');
	let busy = $state(false);
	let error = $state('');
	let dialog: HTMLDialogElement;

	$effect(() => {
		dialog.showModal();
	});

	const monogram = $derived(name.trim().charAt(0).toUpperCase() || '?');
	const noPorts = $derived(container.ports.length === 0);

	async function submit() {
		busy = true;
		error = '';
		try {
			const created = await splitService(fetch, service.slug, {
				service: composeService,
				name,
				category
			});
			let saved = false;
			if (address.trim()) {
				// Vérifiée comme toute adresse ; refusée, la fiche du service la redemande.
				saved = await saveServiceUrl(fetch, created.slug, { url: address.trim() })
					.then((r) => r.saved)
					.catch(() => false);
			}
			ondone(created, saved || !address.trim());
			dialog.close();
		} catch (err) {
			error = err instanceof ApiError ? err.message : String(err);
			busy = false;
		}
	}
</script>

<dialog bind:this={dialog} aria-labelledby="split-title" aria-describedby="split-desc" {onclose}>
	<form
		onsubmit={(e) => {
			e.preventDefault();
			void submit();
		}}
	>
		<span class="grabber" aria-hidden="true"></span>
		<div class="intro">
			<h2 id="split-title">Faire de {composeService} un service à part</h2>
			<p id="split-desc" class="muted">
				Il aura sa propre fiche, son adresse, son état et ses widgets. {service.name} garde {others.join(
					', '
				)}. Rien ne change dans la stack Docker.
			</p>
		</div>

		<label>
			Nom
			<input bind:value={name} type="text" maxlength="80" autocomplete="off" required />
		</label>
		<div class="pair">
			<label>
				Catégorie
				<input
					bind:value={category}
					type="text"
					list="split-categories"
					maxlength="40"
					autocomplete="off"
					required
				/>
				<datalist id="split-categories">
					{#each CATEGORIES as c (c)}
						<option value={c}></option>
					{/each}
				</datalist>
			</label>
			<div class="field">
				<span>Icône</span>
				<div class="icon-row">
					{#key monogram}
						<ServiceIcon icon={null} {monogram} color={service.color} size={40} />
					{/key}
					<span class="muted help">Monogramme, modifiable ensuite</span>
				</div>
			</div>
		</div>
		<label>
			<span>Adresse <span class="muted optional">· facultative</span></span>
			<input
				bind:value={address}
				type="text"
				inputmode="url"
				autocomplete="off"
				spellcheck="false"
				placeholder="https://service.exemple.fr"
				aria-describedby="split-url-help"
			/>
			<span id="split-url-help" class="muted help"
				>{noPorts
					? 'Aucun port publié : sans adresse, le service sera « à configurer ».'
					: 'Sans adresse, le service sera « à configurer ».'} Hub la vérifie avant de la garder.</span
			>
		</label>

		<div class="facts">
			<span
				><b>Conservé :</b> ce choix tient aux redémarrages, aux rescans et aux recréations du conteneur.</span
			>
			<span
				><b>Historique :</b> les événements passés restent sur {service.name}, les suivants vont au
				nouveau service.</span
			>
			<span
				><b>Réversible :</b> « Rattacher à {service.name} » depuis la fiche du nouveau service.</span
			>
		</div>

		<p class="error" role="alert">{error}</p>
		<div class="actions">
			<button type="button" class="btn ghost" disabled={busy} onclick={() => dialog.close()}
				>Annuler</button
			>
			<button type="submit" class="btn primary" disabled={busy}
				>{busy ? 'Création…' : 'Créer le service'}</button
			>
		</div>
	</form>
</dialog>

<style>
	dialog {
		width: min(560px, calc(100vw - 32px));
		max-height: calc(100dvh - 32px);
		box-sizing: border-box;
		padding: 32px;
		border: 1px solid var(--border-2);
		border-radius: 28px;
		background: var(--glass-dialog);
		color: var(--text);
		box-shadow: var(--shadow-lift);
		-webkit-backdrop-filter: var(--blur-strong);
		backdrop-filter: var(--blur-strong);
	}
	dialog::backdrop {
		background: var(--scrim);
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.grabber {
		display: none;
	}
	.intro {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 700;
		overflow-wrap: anywhere;
	}
	.intro p {
		margin: 0;
		font-size: 15px;
		line-height: 1.5;
	}
	.pair {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
	}
	label,
	.field {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 14px;
		font-weight: 500;
	}
	input {
		height: 48px;
		min-width: 0;
		box-sizing: border-box;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--field-bg);
		color: var(--text);
		font: inherit;
		font-size: 16px;
		font-weight: 400;
	}
	.icon-row {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 48px;
	}
	.optional,
	.help {
		font-size: 13px;
		font-weight: 400;
	}
	.facts {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 14px 16px;
		border-radius: 14px;
		background: var(--surface-1);
		color: var(--text-2);
		font-size: 14px;
		line-height: 1.5;
	}
	.facts b {
		color: var(--text);
		font-weight: 600;
	}
	.error {
		margin: 0;
		color: var(--text-danger);
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 10px;
	}
	.btn {
		min-height: 48px;
		padding: 0 20px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-pill);
		background: transparent;
		color: var(--text);
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.primary {
		border-color: transparent;
		background: var(--text);
		color: var(--text-inverse);
	}
	@media (max-width: 720px) {
		dialog {
			width: 100vw;
			max-width: none;
			margin: auto 0 0;
			padding: 10px 16px 28px;
			border-radius: 28px 28px 0 0;
		}
		.grabber {
			display: block;
			align-self: center;
			width: 40px;
			height: 5px;
			border-radius: 99px;
			background: var(--border-dashed);
		}
		.pair {
			grid-template-columns: minmax(0, 1fr);
		}
		.actions {
			flex-direction: column-reverse;
		}
	}
</style>
