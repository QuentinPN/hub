<script lang="ts">
	import { getHostContainers } from '#lib/api/client.ts';
	import type { Container } from '#lib/api/types.ts';
	import Skeleton from './Skeleton.svelte';

	interface Props {
		hostId: string;
		id: string;
		/** Rechargé à chaque changement : la liste des hôtes le fait avancer à chaque relecture. */
		version: number;
		/** L'hôte ne répond plus : ce qui suit est le dernier relevé. */
		stale: boolean;
		onrevoke?: () => void;
	}
	let { hostId, id, version, stale, onrevoke }: Props = $props();

	let containers = $state<Container[] | null>(null);
	let error = $state('');

	$effect(() => {
		void version;
		getHostContainers(fetch, hostId)
			.then((list) => {
				containers = list;
				error = '';
			})
			.catch((err: unknown) => {
				error = `Impossible de charger les conteneurs : ${err instanceof Error ? err.message : String(err)}`;
			});
	});

	const labels = (c: Container) =>
		`${c.labels} label${c.labels > 1 ? 's' : ''}` +
		(c.maskedLabels ? `, ${c.maskedLabels} masqué${c.maskedLabels > 1 ? 's' : ''}` : '');
	const running = (c: Container) => c.state === 'running';
</script>

<div class="box" {id}>
	{#if containers === null && !error}
		<div class="pad" aria-busy="true"><Skeleton height="44px" radius="12px" /></div>
	{:else if error}
		<p class="pad error" role="alert">{error}</p>
	{:else if containers && containers.length === 0}
		<p class="pad muted">Aucun conteneur sur cet hôte.</p>
	{:else if containers}
		{#if stale}
			<p class="pad muted small">Dernier relevé de l’agent : l’état réel peut avoir changé.</p>
		{/if}
		<table>
			<thead>
				<tr
					><th scope="col">Conteneur</th><th scope="col">Projet Compose</th><th scope="col"
						>État et labels</th
					></tr
				>
			</thead>
			<tbody>
				{#each containers as c (c.id)}
					<tr>
						<td>
							<span class="name">{c.name}</span>
							<span class="muted mono image">{c.image}</span>
						</td>
						<td class="mono muted">{c.composeDir || c.composeProject || '—'}</td>
						<td>
							<span class="dot" class:on={running(c) && !stale} aria-hidden="true"></span>{running(
								c
							)
								? 'En marche'
								: c.status || c.state} · {labels(c)}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
	<div class="foot">
		<span class="muted small"
			>Un label masqué garde sa clé, jamais sa valeur. Les variables d’environnement et la commande
			ne quittent pas l’hôte.</span
		>
		{#if onrevoke}
			<button type="button" class="btn danger-ghost" onclick={onrevoke}>Révoquer l’agent</button>
		{/if}
	</div>
</div>

<style>
	.box {
		margin: 0 0 8px;
		overflow: hidden;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: rgba(0, 0, 0, 0.25);
	}
	.pad {
		margin: 0;
		padding: 14px 16px;
	}
	.small {
		font-size: var(--fs-small);
		line-height: 1.5;
	}
	.error {
		color: #ff8a80;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 14px;
	}
	th {
		padding: 12px 16px;
		color: var(--text-2);
		font-size: 12px;
		font-weight: 600;
		text-align: left;
	}
	td {
		padding: 12px 16px;
		border-top: 1px solid var(--border-1);
		vertical-align: top;
		overflow-wrap: anywhere;
	}
	.name {
		display: block;
		font-weight: 600;
	}
	.image {
		font-size: 12px;
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		margin-right: 8px;
		border-radius: 50%;
		background: var(--status-unknown);
	}
	.dot.on {
		background: var(--status-online);
	}
	.foot {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 14px 16px;
		border-top: 1px solid var(--border-1);
	}
	.foot > span {
		flex: 1 1 320px;
	}
	.btn {
		min-height: 44px;
		padding: 0 18px;
		border-radius: 12px;
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.danger-ghost {
		border: 1px solid rgba(255, 138, 128, 0.35);
		background: transparent;
		color: #ff8a80;
	}
	@media (max-width: 720px) {
		/* Une carte par conteneur : trois colonnes ne tiennent pas sur 390 px. */
		thead {
			display: none;
		}
		tr,
		td {
			display: block;
		}
		tr {
			padding: 10px 0;
			border-top: 1px solid var(--border-1);
		}
		td {
			padding: 2px 16px;
			border: 0;
		}
		.btn {
			width: 100%;
		}
	}
</style>
