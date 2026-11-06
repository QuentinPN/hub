<script lang="ts">
	import { ApiError, resetService, updateService } from '#lib/api/client.ts';
	import type { ProjectRef, ServiceDetail } from '#lib/api/types.ts';
	import ServiceIcon from './ServiceIcon.svelte';

	let {
		service,
		onclose,
		onsaved
	}: {
		service: ServiceDetail;
		onclose: () => void;
		onsaved: (service: ServiceDetail) => void;
	} = $props();

	const CATEGORIES = ['Médias', 'Téléchargements', 'Administration', 'Mes applications', 'Autres'];

	// Le panneau part des valeurs à l'ouverture : c'est le rôle de cette copie.
	const initial = () => service;
	let name = $state(initial().name);
	let category = $state(initial().category);
	let icon = $state(initial().icon ?? '');
	let hidden = $state(initial().hidden ?? false);
	let links = $state<ProjectRef[]>([...(initial().links ?? [])]);
	let linkable = $state<ProjectRef[]>([...(initial().linkable ?? [])]);

	let busy = $state(false);
	let error = $state('');
	let confirmReset = $state(false);
	let picked = $state('');
	let dialog: HTMLDialogElement;

	$effect(() => {
		dialog.showModal();
	});

	function link() {
		const project = linkable.find((p) => p.key === picked);
		if (!project) return;
		links = [...links, project];
		linkable = linkable.filter((p) => p.key !== picked);
		picked = '';
	}

	function unlink(project: ProjectRef) {
		links = links.filter((p) => p.key !== project.key);
		linkable = [...linkable, project].sort((a, b) => a.key.localeCompare(b.key));
	}

	const count = (n: number) => `${n} conteneur${n > 1 ? 's' : ''}`;

	async function run(action: () => Promise<ServiceDetail>) {
		busy = true;
		error = '';
		try {
			onsaved(await action());
			dialog.close();
		} catch (err) {
			error = err instanceof ApiError ? err.message : String(err);
			busy = false;
		}
	}

	const save = () =>
		run(() =>
			updateService(fetch, service.slug, {
				name,
				category,
				icon: icon.trim(),
				hidden,
				links: links.map((p) => p.key)
			})
		);

	function reset() {
		if (!confirmReset) {
			confirmReset = true;
			return;
		}
		void run(() => resetService(fetch, service.slug));
	}
</script>

<dialog bind:this={dialog} aria-labelledby="edit-title" {onclose}>
	<form
		method="dialog"
		onsubmit={(e) => {
			e.preventDefault();
			void save();
		}}
	>
		<header>
			<h2 id="edit-title">Modifier {service.name}</h2>
			<button type="button" class="close" aria-label="Fermer" onclick={() => dialog.close()}
				>×</button
			>
		</header>

		<div class="body">
			<div class="pair">
				<label>
					Nom
					<input bind:value={name} type="text" maxlength="80" autocomplete="off" required />
				</label>
				<label>
					Catégorie
					<input
						bind:value={category}
						type="text"
						list="edit-categories"
						maxlength="40"
						autocomplete="off"
						required
					/>
					<datalist id="edit-categories">
						{#each CATEGORIES as c (c)}
							<option value={c}></option>
						{/each}
					</datalist>
				</label>
			</div>

			<div class="field">
				<label for="edit-icon">Icône</label>
				<div class="icon-row">
					{#key icon}
						<ServiceIcon
							icon={icon.trim() || null}
							monogram={name.trim().charAt(0).toUpperCase() || '?'}
							color={service.color}
							size={48}
						/>
					{/key}
					<input
						id="edit-icon"
						bind:value={icon}
						type="text"
						autocomplete="off"
						spellcheck="false"
						placeholder="Nom selfh.st, ex. emby"
						aria-describedby="edit-icon-help"
					/>
				</div>
				<span id="edit-icon-help" class="muted help">Sans icône, Hub garde la première lettre.</span
				>
			</div>

			<fieldset>
				<legend>Conteneurs du service</legend>
				{#if service.project}
					<div class="project">
						<span
							>{service.project.key}
							<span class="muted">· {count(service.project.containers)}</span></span
						>
						<span class="muted origin">projet d’origine</span>
					</div>
				{/if}
				{#each links as project (project.key)}
					<div class="project linked">
						<span>{project.key} <span class="muted">· {count(project.containers)}</span></span>
						<button
							type="button"
							class="link"
							aria-label="Retirer {project.key}"
							onclick={() => unlink(project)}>Retirer</button
						>
					</div>
				{/each}
				{#if linkable.length > 0}
					<label>
						Relier un autre projet de {service.hostId}
						<select bind:value={picked} onchange={link}>
							<option value="">Choisir un projet</option>
							{#each linkable as project (project.key)}
								<option value={project.key}>{project.key}</option>
							{/each}
						</select>
					</label>
				{/if}
				<span class="muted help"
					>Un conteneur relié compte comme appoint : s’il s’arrête, le service passe en « dégradé ».</span
				>
			</fieldset>

			<label class="toggle">
				<span>
					Masquer ce service
					<span class="muted help">Il reste listé dans « Masqués » et dans Hôtes et agents.</span>
				</span>
				<input bind:checked={hidden} type="checkbox" role="switch" />
			</label>

			<p class="error" role="alert">{error}</p>
		</div>

		<footer>
			<button type="button" class="link quiet" disabled={busy} onclick={reset}>
				{confirmReset ? 'Confirmer : effacer mes corrections' : 'Reprendre le détecté'}
			</button>
			<span class="buttons">
				<button type="button" class="btn" disabled={busy} onclick={() => dialog.close()}
					>Annuler</button
				>
				<button type="submit" class="btn primary" disabled={busy}>
					{busy ? 'Enregistrement…' : 'Enregistrer'}
				</button>
			</span>
		</footer>
	</form>
</dialog>

<style>
	dialog {
		position: fixed;
		inset: 0 0 0 auto;
		width: min(560px, 100vw);
		height: 100dvh;
		max-width: none;
		max-height: none;
		margin: 0;
		padding: 0;
		border: 0;
		border-left: 1px solid var(--border-2);
		background: #121418;
		color: var(--text);
	}
	dialog::backdrop {
		background: rgba(0, 0, 0, 0.6);
	}
	form {
		display: flex;
		flex-direction: column;
		height: 100%;
	}
	header,
	footer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 18px 28px;
	}
	footer {
		border-top: 1px solid var(--border-1);
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.close {
		width: 44px;
		height: 44px;
		border: 0;
		border-radius: 12px;
		background: var(--surface-2);
		color: var(--text);
		font-size: 20px;
		cursor: pointer;
	}
	.body {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 18px;
		min-height: 0;
		padding: 4px 28px 20px;
		overflow-y: auto;
	}
	.pair {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 14px;
	}
	label,
	.field {
		display: flex;
		flex-direction: column;
		gap: 7px;
		font-size: 14px;
		font-weight: 500;
	}
	input[type='text'],
	select {
		height: 48px;
		min-width: 0;
		box-sizing: border-box;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: #1c1f24;
		color: var(--text);
		font: inherit;
		font-size: 16px;
		font-weight: 400;
	}
	.icon-row {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.icon-row input {
		flex: 1;
	}
	.help {
		font-size: 14px;
		font-weight: 400;
	}
	fieldset {
		display: flex;
		flex-direction: column;
		gap: 10px;
		margin: 0;
		padding: 0;
		border: 0;
	}
	legend {
		padding: 0 0 8px;
		font-size: 14px;
		font-weight: 500;
	}
	.project {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 4px 12px;
		min-height: 44px;
		box-sizing: border-box;
		padding: 4px 8px 4px 14px;
		border: 1px solid var(--border-1);
		border-radius: 12px;
		font-size: 15px;
		overflow-wrap: anywhere;
	}
	.linked {
		border-color: rgba(232, 201, 166, 0.3);
		background: rgba(232, 201, 166, 0.07);
	}
	.origin {
		font-size: 14px;
	}
	.toggle {
		flex-direction: row;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		min-height: 48px;
		font-size: 15px;
	}
	.toggle span {
		display: flex;
		flex-direction: column;
		gap: 3px;
	}
	.toggle input {
		flex: none;
		width: 24px;
		height: 24px;
		accent-color: var(--accent-play);
	}
	.error {
		margin: 0;
		color: #ff8a80;
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
	.buttons {
		display: flex;
		gap: 10px;
	}
	.btn {
		min-height: 48px;
		padding: 0 20px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-button);
		background: transparent;
		color: var(--text);
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.primary {
		border-color: transparent;
		background: var(--text);
		color: var(--text-inverse);
	}
	.link {
		min-height: 44px;
		padding: 0 8px;
		border: 0;
		background: transparent;
		color: var(--text);
		font-size: 14px;
		text-decoration: underline;
		cursor: pointer;
	}
	.quiet {
		padding: 0 4px;
		color: var(--text-2);
	}
	@media (max-width: 720px) {
		dialog {
			inset: 44px 0 0 0;
			width: 100vw;
			height: calc(100dvh - 44px);
			border-top: 1px solid var(--border-2);
			border-left: 0;
			border-radius: 24px 24px 0 0;
		}
		header,
		footer {
			padding: 14px 16px;
		}
		.body {
			padding: 4px 16px 12px;
		}
		.pair {
			grid-template-columns: minmax(0, 1fr);
		}
		.buttons {
			flex: 1 1 100%;
		}
		.buttons .btn {
			flex: 1;
		}
	}
</style>
