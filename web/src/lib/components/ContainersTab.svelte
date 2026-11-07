<script module lang="ts">
	// Passe à true quand l'onglet est prêt : la fiche le rend alors cliquable.
	export const ready = true;
</script>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, getServiceContainers } from '#lib/api/client.ts';
	import type { ServiceContainer, ServiceContainers, ServiceDetail } from '#lib/api/types.ts';
	import { agentStateView, ago } from '#lib/agents.ts';
	import {
		byProject,
		formatDate,
		healthView,
		labelsLabel,
		portLabel,
		roleLabels,
		splitImage,
		splittable,
		stateColor
	} from '#lib/containers.ts';
	import { widgetPollMs } from '#lib/shell.svelte.ts';
	import ErrorPanel from './ErrorPanel.svelte';
	import JoinDialog from './JoinDialog.svelte';
	import Skeleton from './Skeleton.svelte';
	import SplitDialog from './SplitDialog.svelte';

	let { service }: { service: ServiceDetail } = $props();

	let data = $state<ServiceContainers | null>(null);
	/** Le service n'a pas été découvert par un agent : il n'a pas de conteneurs. */
	let undiscovered = $state(false);
	let error = $state('');
	let now = $state(Date.now());

	async function load(slug: string) {
		try {
			data = await getServiceContainers(fetch, slug);
			undiscovered = false;
			error = '';
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) {
				undiscovered = true;
				error = '';
			} else if (!data) {
				error = err instanceof Error ? err.message : String(err);
			}
		}
	}

	// Lecture seule, relue au rythme des widgets (Réglages > Général > Rafraîchissement).
	$effect(() => {
		const slug = service.slug;
		void load(slug);
		const poll = setInterval(() => {
			if (document.visibilityState === 'visible') void load(slug);
		}, widgetPollMs());
		const tick = setInterval(() => (now = Date.now()), 5_000);
		return () => {
			clearInterval(poll);
			clearInterval(tick);
		};
	});

	const hostKnown = $derived(data?.hostState === 'connected');
	const groups = $derived(data ? byProject(data.containers) : []);
	const running = $derived(data?.containers.filter((c) => c.state === 'running').length ?? 0);
	const plural = (n: number, one: string, many: string) => `${n} ${n > 1 ? many : one}`;

	// Mettre à part un service Compose, ou rattacher celui-ci à son projet.
	const canSplit = $derived(data ? splittable(data) : new Set<string>());
	const splitKey = $derived(data?.split ? `${data.split.project}/${data.split.service}` : '');
	let splitting = $state<ServiceContainer | null>(null);
	let joining = $state(false);

	/** Ouvre un onglet d'une fiche. */
	function openTab(slug: string, tab: 'apercu' | 'conteneurs') {
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- chemin déjà résolu, seule l'ancre s'ajoute
		void goto(`${resolve('/services/[slug]', { slug })}#${tab}`);
	}

	function compose(c: ServiceContainer): string {
		if (!c.composeProject) return 'Hors Compose';
		return c.composeService ? `${c.composeProject} · ${c.composeService}` : c.composeProject;
	}
</script>

{#if error}
	<ErrorPanel
		title="Impossible de charger les conteneurs"
		detail={error}
		onretry={() => load(service.slug)}
	/>
{:else if undiscovered}
	<div class="empty">
		<svg
			width="28"
			height="28"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.8"
			stroke-linejoin="round"
			aria-hidden="true"
			><path d="M3 7.5 12 3l9 4.5v9L12 21l-9-4.5z"></path><path d="M3 7.5 12 12l9-4.5M12 12v9"
			></path></svg
		>
		<h2>Aucun conteneur</h2>
		<p class="muted">
			Ce service n'a pas été découvert par un agent Docker : Hub ne lui connaît aucun conteneur.
		</p>
	</div>
{:else if !data}
	<div class="loading" aria-busy="true" aria-label="Chargement des conteneurs">
		<Skeleton width="min(420px, 70%)" height="18px" radius="8px" />
		<div class="grid">
			{#each [0, 1, 2] as i (i)}
				<Skeleton height="260px" radius="var(--r-card)" />
			{/each}
		</div>
	</div>
{:else}
	{@const host = data.hostName || data.hostId}
	<div class="tab">
		{#if hostKnown}
			<p class="summary muted">
				<span class="dot" style:background="var(--status-online)" aria-hidden="true"></span>
				<span
					><b>{plural(data.containers.length, 'conteneur', 'conteneurs')}</b>
					{`sur ${host} · ${running} en marche`}{data.lastSeenAt
						? ` · agent connecté, dernier contact ${ago(data.lastSeenAt, now)}`
						: ''}</span
				>
			</p>
		{:else}
			<div class="banner" role="status">
				<svg
					width="20"
					height="20"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
					><path d="M12 3 2 20h20z"></path><path d="M12 10v4M12 17.5v.01"></path></svg
				>
				<div>
					<b>Agent de {host} : {agentStateView(data.hostState).label.toLowerCase()}</b>
					<span class="muted"
						>{data.lastSeenAt ? `Dernier contact ${ago(data.lastSeenAt, now)}. ` : ''}Les états
						ci-dessous datent de ce moment.</span
					>
				</div>
			</div>
		{/if}

		{#if data.containers.length === 0}
			<div class="empty">
				<h2>Aucun conteneur</h2>
				<p class="muted">Ses conteneurs ne sont plus sur {host}.</p>
			</div>
		{/if}

		{#each groups as g (g.key)}
			<section class="project" aria-labelledby="project-{g.key}">
				{#if data.split && g.key === splitKey}
					{@const sp = data.split}
					<div class="head">
						<h2 id="project-{g.key}">{sp.project}</h2>
						<span class="muted"
							>séparé du projet le {formatDate(sp.since)}{#if sp.parentSlug}&nbsp;· le reste du
								projet est sur
								<a href="{resolve('/services/[slug]', { slug: sp.parentSlug })}#conteneurs"
									>{sp.parentName}</a
								>{/if}</span
						>
						<button type="button" class="action join" onclick={() => (joining = true)}>
							<svg
								width="16"
								height="16"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
								><path d="M8 3 4 7l4 4"></path><path d="M4 7h16"></path><path d="m16 21 4-4-4-4"
								></path><path d="M20 17H4"></path></svg
							>
							Rattacher à {sp.parentName || sp.project}…
						</button>
					</div>
				{:else}
					<div class="head">
						<h2 id="project-{g.key}">{g.key}</h2>
						<span class="muted"
							>{g.linked ? 'projet relié' : 'projet du service'} · {plural(
								g.containers.length,
								'conteneur',
								'conteneurs'
							)}</span
						>
					</div>
				{/if}
				<div class="grid">
					{#each g.containers as c (c.id)}
						{@const img = splitImage(c.image)}
						{@const h = healthView(c.health)}
						<article class="card" aria-labelledby="ctr-{c.id}">
							<div class="title">
								<span
									class="dot"
									style:background={stateColor(c, hostKnown)}
									role="img"
									aria-label={c.stateLabel}
								></span>
								<h3 id="ctr-{c.id}">{c.name}</h3>
								{#if c.role}
									<span class="pill" class:main={c.role === 'main'}>{roleLabels[c.role]}</span>
								{/if}
							</div>
							<div class="image">
								<span class="mono">{img.name}</span>
								<span class="mono tag">{img.tag}</span>
							</div>
							<div class="state">
								<span
									><span class="cap">{c.stateLabel}</span>
									<span class="muted">· {c.status}{hostKnown ? '' : ' · au dernier contact'}</span
									></span
								>
								<span class="health {h.tone}">{h.label}</span>
							</div>
							<dl>
								<dt class="muted">Hôte</dt>
								<dd>{host}</dd>
								<dt class="muted">Ports</dt>
								<dd class="mono">
									{c.ports.length ? c.ports.map(portLabel).join(', ') : 'Aucun port publié'}
								</dd>
								<dt class="muted">Compose</dt>
								<dd>{compose(c)}</dd>
								<dt class="muted">Labels</dt>
								<dd>{labelsLabel(c.labels, c.maskedLabels)}</dd>
								<dt class="muted">Vu depuis</dt>
								<dd>{formatDate(c.firstSeenAt)}</dd>
								<dt class="muted">ID</dt>
								<dd class="mono muted">{c.id}</dd>
							</dl>
							{#if !g.linked && canSplit.has(c.composeService)}
								<div class="foot">
									<button type="button" class="action" onclick={() => (splitting = c)}>
										<svg
											width="16"
											height="16"
											viewBox="0 0 24 24"
											fill="none"
											stroke="currentColor"
											stroke-width="2"
											stroke-linecap="round"
											stroke-linejoin="round"
											aria-hidden="true"
											><path d="M16 3h5v5"></path><path d="M8 3H3v5"></path><path
												d="M12 22v-8.3a4 4 0 0 0-1.172-2.872L3 3"
											></path><path d="m15 9 6-6"></path></svg
										>
										En faire un service à part…
									</button>
								</div>
							{/if}
						</article>
					{/each}
				</div>
			</section>
		{/each}

		{#if data.removed.length > 0}
			<section class="project" aria-labelledby="removed-head">
				<div class="head">
					<h2 id="removed-head" class="muted">Disparus</h2>
					<span class="muted"
						>plus sur l'hôte, gardés pour mémoire{#if data.removedTotal > data.removed.length}&nbsp;·
							les {data.removed.length} derniers sur {data.removedTotal}{/if}</span
					>
				</div>
				<div class="grid">
					{#each data.removed as c (c.id)}
						{@const img = splitImage(c.image)}
						<article class="card gone" aria-labelledby="gone-{c.id}">
							<div class="title">
								<span class="dot small" style:background="var(--status-unknown)" aria-hidden="true"
								></span>
								<h3 id="gone-{c.id}">{c.name}</h3>
								{#if c.removedAt}<span class="when">disparu le {formatDate(c.removedAt)}</span>{/if}
							</div>
							<div class="image">
								<span class="mono">{img.name}</span>
								<span class="mono tag">{img.tag}</span>
							</div>
							<span class="detail"
								>{compose(c)} · vu depuis le {formatDate(c.firstSeenAt)} · ID {c.id}</span
							>
						</article>
					{/each}
				</div>
			</section>
		{/if}
	</div>
{/if}

{#if splitting}
	<SplitDialog
		{service}
		container={splitting}
		others={[...canSplit].filter((x) => x !== splitting?.composeService)}
		onclose={() => (splitting = null)}
		ondone={(created, urlSaved) => openTab(created.slug, urlSaved ? 'conteneurs' : 'apercu')}
	/>
{/if}
{#if joining && data?.split}
	<JoinDialog
		{service}
		split={data.split}
		onclose={() => (joining = false)}
		ondone={(parent) => (parent ? openTab(parent.slug, 'conteneurs') : void goto(resolve('/')))}
	/>
{/if}

<style>
	.tab,
	.loading {
		display: flex;
		flex-direction: column;
		gap: 22px;
	}
	.summary {
		display: flex;
		align-items: center;
		gap: 8px;
		margin: 0;
		font-size: 15px;
	}
	.summary b {
		color: var(--text);
		font-weight: 600;
	}
	.dot {
		flex: none;
		width: 10px;
		height: 10px;
		border-radius: 50%;
	}
	.dot.small,
	.summary .dot {
		width: 8px;
		height: 8px;
	}
	.banner {
		display: flex;
		gap: 12px;
		padding: 16px 18px;
		border: 1px solid var(--tint-warn-border);
		border-radius: 16px;
		background: var(--hatch);
		font-size: 14px;
	}
	.banner svg {
		flex: none;
		margin-top: 1px;
		color: var(--text-warn);
	}
	.banner div {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.banner b {
		font-weight: 600;
	}
	.project {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 2px 12px;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.head span {
		font-size: 14px;
	}
	.head a {
		color: var(--text);
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.action {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-height: 44px;
		padding: 0 16px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-pill);
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 14px;
		font-weight: 500;
		cursor: pointer;
	}
	.action:hover {
		background: var(--surface-2);
	}
	.join {
		margin-left: auto;
		align-self: center;
	}
	.foot {
		display: flex;
		margin-top: auto;
		padding-top: 14px;
		border-top: 1px solid var(--border-1);
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
		gap: 20px;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 14px;
		min-width: 0;
		padding: 22px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.title {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	h3 {
		flex: 1;
		min-width: 0;
		margin: 0;
		font-size: 17px;
		font-weight: 600;
		overflow-wrap: anywhere;
	}
	.pill {
		flex: none;
		padding: 3px 10px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-pill);
		color: var(--text-2);
		font-size: 12px;
		font-weight: 600;
	}
	.pill.main {
		border-color: var(--tint-accent-border-strong);
		background: var(--tint-accent-bg-strong);
		color: var(--accent-text);
	}
	.mono {
		font-family: var(--font-mono);
	}
	.image {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px 8px;
		font-size: 14px;
	}
	.image .mono:first-child {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.tag {
		padding: 2px 8px;
		border-radius: 6px;
		background: var(--surface-2);
		font-size: 13px;
	}
	.state {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
		font-size: 14px;
	}
	.cap {
		display: inline-block;
	}
	.cap::first-letter {
		text-transform: uppercase;
	}
	.health {
		padding: 3px 10px;
		border: 1px solid transparent;
		border-radius: var(--r-pill);
		font-size: 12px;
		font-weight: 600;
	}
	.health.ok {
		border-color: var(--tint-ok-border);
		background: var(--tint-ok-bg-strong);
		color: var(--text-ok);
	}
	.health.warn {
		border-color: var(--tint-warn-border);
		background: var(--tint-warn-bg-strong);
		color: var(--text-warn);
	}
	.health.danger {
		border-color: var(--tint-danger-border);
		background: var(--tint-danger-bg-strong);
		color: var(--text-danger);
	}
	.health.none {
		padding: 0;
		color: var(--text-2);
		font-size: 13px;
		font-weight: 400;
	}
	dl {
		display: grid;
		grid-template-columns: 96px minmax(0, 1fr);
		gap: 8px 14px;
		margin: 0;
		padding-top: 14px;
		border-top: 1px solid var(--border-1);
		font-size: 14px;
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	dd.mono {
		font-size: 13px;
	}
	.gone {
		gap: 10px;
		padding: 18px 22px;
		border-style: dashed;
		border-color: var(--border-2);
		background: var(--surface-0);
		color: var(--text-2);
	}
	.gone h3 {
		font-size: 16px;
	}
	.gone .image,
	.when,
	.detail {
		font-size: 13px;
	}
	.gone .tag {
		background: var(--surface-1);
	}
	.empty {
		display: flex;
		flex-direction: column;
		gap: 10px;
		max-width: 520px;
		padding: 28px 24px;
		border: 1px dashed var(--border-dashed);
		border-radius: var(--r-card);
		background: var(--surface-0);
		color: var(--text-2);
	}
	.empty h2 {
		color: var(--text);
		font-size: 17px;
	}
	.empty p {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
	}
	@media (max-width: 720px) {
		.card {
			padding: 18px;
		}
		.foot .action {
			flex: 1;
			justify-content: center;
		}
		dl {
			grid-template-columns: 84px minmax(0, 1fr);
			font-size: 13px;
		}
	}
</style>
