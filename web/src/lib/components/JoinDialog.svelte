<script lang="ts">
	import { ApiError, joinService } from '#lib/api/client.ts';
	import type { ContainerSplit, ServiceDetail } from '#lib/api/types.ts';

	interface Props {
		service: ServiceDetail;
		split: ContainerSplit;
		onclose: () => void;
		/** Rattaché : le service du projet, null s'il n'en a plus. */
		ondone: (parent: ServiceDetail | null) => void;
	}
	let { service, split, onclose, ondone }: Props = $props();

	let dialog: HTMLDialogElement;
	let busy = $state(false);
	let error = $state('');
	const parent = $derived(split.parentName || split.project);

	$effect(() => {
		dialog.showModal();
	});

	async function join() {
		busy = true;
		error = '';
		try {
			ondone(await joinService(fetch, service.slug));
			dialog.close();
		} catch (err) {
			error = err instanceof ApiError ? err.message : String(err);
			busy = false;
		}
	}
</script>

<dialog
	bind:this={dialog}
	role="alertdialog"
	aria-labelledby="join-title"
	aria-describedby="join-desc"
	{onclose}
>
	<span class="grabber" aria-hidden="true"></span>
	<h2 id="join-title">Rattacher {split.service} à {parent} ?</h2>
	<div id="join-desc" class="desc">
		<p>
			Ses conteneurs redeviennent des appoints de {parent}, et {service.name} disparaît du catalogue.
		</p>
		<p class="muted">
			Son historique est gardé. Le séparer de nouveau fait revenir {service.name} avec son adresse et
			ses widgets.
		</p>
	</div>
	<p class="error" role="alert">{error}</p>
	<div class="actions">
		<button type="button" class="btn ghost" disabled={busy} onclick={() => dialog.close()}
			>Annuler</button
		>
		<button type="button" class="btn primary" disabled={busy} onclick={join}
			>{busy ? 'Rattachement…' : 'Rattacher'}</button
		>
	</div>
</dialog>

<style>
	dialog {
		width: min(520px, calc(100vw - 32px));
		padding: 24px;
		border: 1px solid var(--border-2);
		border-radius: 24px;
		background: var(--glass-dialog);
		color: var(--text);
		box-shadow: var(--shadow-lift);
		-webkit-backdrop-filter: var(--blur-strong);
		backdrop-filter: var(--blur-strong);
	}
	dialog::backdrop {
		background: var(--scrim);
	}
	.grabber {
		display: none;
	}
	h2 {
		margin: 0 0 12px;
		font-size: var(--fs-h2);
		font-weight: 600;
		overflow-wrap: anywhere;
	}
	.desc p {
		margin: 0 0 10px;
		font-size: 15px;
		line-height: 1.5;
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
		margin-top: 12px;
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
			padding: 12px 20px 28px;
			border-radius: 28px 28px 0 0;
		}
		.grabber {
			display: block;
			width: 40px;
			height: 5px;
			margin: 0 auto 14px;
			border-radius: 99px;
			background: var(--border-dashed);
		}
		.actions {
			flex-direction: column-reverse;
		}
	}
</style>
