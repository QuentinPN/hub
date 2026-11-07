<script lang="ts">
	import { revokeAgent } from '#lib/api/client.ts';
	import type { AgentHost } from '#lib/api/types.ts';

	interface Props {
		host: AgentHost;
		onclose: () => void;
		/** Révocation faite : la liste se recharge et annonce le résultat. */
		ondone: (message: string) => void;
	}
	let { host, onclose, ondone }: Props = $props();

	let dialog: HTMLDialogElement;
	let busy = $state(false);
	let error = $state('');

	$effect(() => {
		dialog.showModal();
	});

	async function revoke() {
		busy = true;
		error = '';
		try {
			await revokeAgent(fetch, host.id);
			ondone(`Agent de ${host.name} révoqué. Il est refusé dès sa prochaine requête.`);
			dialog.close();
		} catch (err) {
			error = `Échec de la révocation : ${err instanceof Error ? err.message : String(err)}`;
			busy = false;
		}
	}
</script>

<dialog
	bind:this={dialog}
	role="alertdialog"
	aria-labelledby="revoke-title"
	aria-describedby="revoke-desc"
	{onclose}
>
	<span class="grabber" aria-hidden="true"></span>
	<h2 id="revoke-title">Révoquer l’agent de {host.name} ?</h2>
	<div id="revoke-desc" class="desc">
		<p>L’agent est refusé dès sa prochaine requête, dans les 15 s.</p>
		<p class="muted">
			{host.containers > 1
				? `Les ${host.containers} conteneurs restent`
				: host.containers === 1
					? 'Le conteneur reste'
					: 'Les conteneurs connus restent'} dans l’inventaire avec leurs personnalisations, affichés
			« inconnu ». Sur l’hôte, rien n’est arrêté : l’agent continue d’essayer, en vain, jusqu’à ce que
			tu le retires.
		</p>
		<p class="muted">Pour le reconnecter : un nouveau code d’enrôlement.</p>
	</div>
	<p class="error" role="alert">{error}</p>
	<div class="actions">
		<button type="button" class="btn ghost" disabled={busy} onclick={() => dialog.close()}
			>Annuler</button
		>
		<button type="button" class="btn danger" disabled={busy} onclick={revoke}
			>{busy ? 'Révocation…' : 'Révoquer l’agent'}</button
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
		padding: 0 18px;
		border: 0;
		border-radius: 14px;
		font-size: 16px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.ghost {
		border: 1px solid var(--border-2);
		background: transparent;
		color: var(--text);
	}
	.danger {
		background: var(--danger-fill);
		color: var(--on-danger);
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
