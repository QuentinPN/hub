<script lang="ts">
	import { resolve } from '$app/paths';
	import { getInfraPage } from '#lib/api/client.ts';
	import type { InfraPage } from '#lib/api/types.ts';
	import { agentStateView, ago } from '#lib/agents.ts';
	import SectionPage from '#lib/components/SectionPage.svelte';
	import StatusDot from '#lib/components/StatusDot.svelte';
	import { statusStyle } from '#lib/status.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const plural = (n: number, one: string, many: string) => `${n} ${n > 1 ? many : one}`;

	function lead(p: InfraPage): string {
		const connected = p.hosts.filter((h) => h.state === 'connected').length;
		const containers = p.hosts.reduce((sum, h) => sum + h.containers, 0);
		// « Inconnu » n'est pas « sans problème » : l'hôte ne dit plus rien de ses services.
		const fine = p.groups
			.flatMap((g) => g.services)
			.filter((s) => s.status === 'online' || s.status === 'config_required').length;
		return [
			plural(connected, 'hôte connecté', 'hôtes connectés'),
			plural(containers, 'conteneur', 'conteneurs'),
			`${fine} services sur ${p.total} sans problème`
		].join(' · ');
	}
</script>

<SectionPage
	title="Infra"
	tint="#1e3a33"
	loaded={data.page}
	reload={() => getInfraPage(fetch)}
	{lead}
>
	{#snippet children(page: InfraPage)}
		{#if page.watch.length > 0}
			<section class="card watch" aria-labelledby="watch">
				<h2 id="watch">À surveiller</h2>
				{#each page.watch as s (s.slug)}
					<a class="watched" href={resolve('/services/[slug]', { slug: s.slug })}>
						<strong>{s.name}</strong>
						<span class="muted state"
							><StatusDot status={s.status} size={8} />{statusStyle(s.status).label} · {s.note} · {s.hostId}</span
						>
					</a>
				{/each}
			</section>
		{/if}

		<section aria-labelledby="hosts">
			<h2 id="hosts" class="title">Hôtes</h2>
			<div class="hosts">
				{#each page.hosts as h (h.id)}
					{@const view = agentStateView(h.state)}
					<article class="card" aria-label={h.name}>
						<div class="head">
							<strong class="name">{h.name}</strong>
							<span class="state small"
								><span class="dot" style:background={view.color}></span>{view.label}</span
							>
						</div>
						<dl>
							<dt class="muted">Conteneurs</dt>
							<dd class="tabular">{h.containers}</dd>
							<dt class="muted">Services</dt>
							<dd class="tabular">{h.services}</dd>
							{#if h.os}
								<dt class="muted">Système</dt>
								<dd>{h.os}</dd>
							{/if}
							{#if h.dockerVersion}
								<dt class="muted">Docker</dt>
								<dd>{h.dockerVersion}</dd>
							{/if}
							{#if h.lastSeenAt}
								<dt class="muted">Vu</dt>
								<dd>{ago(h.lastSeenAt, Date.now())}</dd>
							{/if}
						</dl>
					</article>
				{:else}
					<p class="muted">
						Aucun hôte n’a encore parlé. <a href="{resolve('/settings')}#hosts">Connecter un hôte</a
						>
					</p>
				{/each}
			</div>
			{#if page.hosts.length > 0}
				<p class="muted beszel">CPU, mémoire et disque arriveront avec le connecteur Beszel.</p>
			{/if}
		</section>

		{#if page.groups.length > 0}
			<section class="groups" aria-labelledby="by-host">
				<h2 id="by-host">Services par hôte</h2>
				{#each page.groups as g (g.hostId)}
					<div class="group">
						<h3 class="muted">{g.name}</h3>
						<div class="chips">
							{#each g.services as s (s.slug)}
								<a
									class="chip"
									href={resolve('/services/[slug]', { slug: s.slug })}
									aria-label="{s.name} : {statusStyle(s.status).label}"
								>
									<StatusDot status={s.status} size={8} />{s.name}
								</a>
							{/each}
						</div>
					</div>
				{/each}
			</section>
		{/if}
	{/snippet}
</SectionPage>

<style>
	.card {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 20px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.watch {
		border-color: rgba(255, 214, 10, 0.3);
		background: rgba(255, 214, 10, 0.06);
	}
	.watched {
		display: flex;
		flex-direction: column;
		gap: 3px;
	}
	.watched strong {
		font-weight: 600;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.title {
		margin-bottom: 14px;
	}
	h3 {
		margin: 0 0 10px;
		font-size: 14px;
		font-weight: 500;
	}
	.hosts {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 18px;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 6px 12px;
	}
	.name {
		font-size: 20px;
		font-weight: 600;
	}
	.state {
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}
	.small {
		font-size: 14px;
	}
	.dot {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 99px;
	}
	dl {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 10px 16px;
		margin: 0;
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
		text-align: right;
	}
	p {
		margin: 0;
	}
	p a {
		text-decoration: underline;
	}
	.beszel {
		margin-top: 14px;
	}
	.groups {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.chips {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
		gap: 10px;
	}
	.chip {
		display: flex;
		align-items: center;
		gap: 9px;
		min-height: 48px;
		box-sizing: border-box;
		padding: 6px 12px;
		border: 1px solid var(--border-1);
		border-radius: 14px;
		background: var(--surface-1);
		overflow-wrap: anywhere;
	}
	@media (max-width: 720px) {
		.groups {
			display: none;
		}
	}
</style>
