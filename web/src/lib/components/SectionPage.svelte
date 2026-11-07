<script lang="ts" generics="T">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { Snippet } from 'svelte';
	import type { SectionBase } from '#lib/api/types.ts';
	import { ago } from '#lib/agents.ts';
	import { onWidgetNews } from '#lib/live.ts';
	import { widgetStateView } from '#lib/widgets.ts';
	import ErrorPanel from './ErrorPanel.svelte';
	import ServiceIcon from './ServiceIcon.svelte';
	import Skeleton from './Skeleton.svelte';

	let {
		title,
		tint,
		loaded,
		reload,
		base,
		lead,
		head,
		children
	}: {
		title: string;
		tint: string;
		/** La première lecture, faite par le chargement de la page. */
		loaded: Promise<T>;
		/** Relit la page sans repasser par le squelette. */
		reload: () => Promise<T>;
		/** Ce qui dit l'état des connexions ; une page sans connexion n'en a pas. */
		base?: (page: T) => SectionBase;
		lead: (page: T) => string;
		head?: Snippet<[T]>;
		children: Snippet<[T]>;
	} = $props();

	let fresh = $state<T | null>(null);

	// Au signal du serveur, et à intervalle régulier pour rattraper un signal perdu.
	$effect(() => {
		const refresh = () => {
			if (document.visibilityState !== 'visible') return;
			reload()
				.then((page) => (fresh = page))
				.catch(() => {});
		};
		const poll = setInterval(refresh, 30_000);
		const stop = onWidgetNews(refresh);
		return () => {
			clearInterval(poll);
			stop();
		};
	});
</script>

<svelte:head><title>{title} · Hub</title></svelte:head>

<section class="hero" style:--tint={tint}>
	<div class="row">
		<div class="text">
			<h1>{title}</h1>
			{#await loaded}
				<Skeleton width="280px" height="18px" />
			{:then first}
				<p class="muted lead">{lead(fresh ?? first)}</p>
			{:catch}
				<span></span>
			{/await}
		</div>
		{#await loaded then first}
			{@render head?.(fresh ?? first)}
		{:catch}
			<span></span>
		{/await}
	</div>
</section>

<div class="content">
	{#await loaded}
		<div aria-busy="true" class="busy">
			<Skeleton height="160px" radius="20px" />
			<Skeleton height="220px" radius="20px" />
		</div>
	{:then first}
		{@const page = fresh ?? first}
		{@const state = base?.(page)}
		{#each state?.notices ?? [] as notice (notice.slug)}
			{@const view = widgetStateView(notice.state)}
			<a
				class="notice {view.tone}"
				href="{resolve('/services/[slug]', { slug: notice.slug })}#widgets"
			>
				<span class="dot" style:background={view.color}></span>
				<span>
					<strong>{notice.name} : {view.label.toLowerCase()}</strong>
					{#if notice.dataAt && view.tone === 'warn'}
						<span class="muted">, données {ago(notice.dataAt, Date.now())}</span>
					{/if}
				</span>
			</a>
		{/each}
		{#if state && state.connected === 0}
			<p class="muted empty">
				Rien de connecté pour l’instant. Hub remplit cette page dès qu’un de ces services a sa clé.
			</p>
			{#each state.toConnect as row (row.slug)}
				<div class="connect">
					<ServiceIcon icon={row.icon} monogram={row.monogram} color={row.color} size={44} />
					<strong>{row.name}</strong>
					<a class="btn" href="{resolve('/services/[slug]', { slug: row.slug })}#widgets"
						>Connecter {row.name}</a
					>
				</div>
			{:else}
				<p class="muted">Aucun service reconnu pour cette page sur tes hôtes.</p>
			{/each}
		{:else}
			{@render children(page)}
		{/if}
	{:catch err}
		<ErrorPanel
			title="Impossible de charger la page"
			detail={err instanceof Error ? err.message : String(err)}
			onretry={() => invalidateAll()}
		/>
	{/await}
</div>

<style>
	.hero {
		padding: calc(var(--shell-h) + 48px) var(--space-page) 30px;
		background:
			radial-gradient(
				90% 100% at 78% 0%,
				color-mix(in srgb, var(--tint) var(--tint-mix), var(--bg)) 0%,
				transparent 62%
			),
			linear-gradient(180deg, var(--hero-fade) 0%, var(--bg) 100%);
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: 24px 32px;
	}
	.text {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}
	h1 {
		margin: 0;
		font-size: clamp(2.2rem, 5vw, 3.5rem);
		font-weight: 700;
		line-height: 1.02;
		letter-spacing: -0.025em;
		overflow-wrap: anywhere;
	}
	.lead {
		margin: 0;
		font-size: 17px;
	}
	.content {
		display: flex;
		flex-direction: column;
		gap: 22px;
		padding: 4px var(--space-page) 64px;
	}
	.busy {
		display: flex;
		flex-direction: column;
		gap: 22px;
	}
	.notice {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 12px 16px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-1);
	}
	.notice strong {
		font-weight: 600;
	}
	.warn {
		border-color: var(--tint-warn-border);
		background: var(--tint-warn-bg);
	}
	.bad {
		border-color: var(--tint-danger-border);
		background: var(--tint-danger-bg);
	}
	.dot {
		flex: none;
		width: 9px;
		height: 9px;
		border-radius: 99px;
	}
	.empty {
		margin: 0;
		max-width: 640px;
		font-size: 17px;
	}
	.connect {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px 14px;
		max-width: 760px;
		padding: 12px 12px 12px 14px;
		border: 1px dashed var(--tint-accent-border-strong);
		border-radius: 18px;
		background: var(--tint-accent-bg);
	}
	.connect strong {
		flex: 1 1 120px;
		font-weight: 600;
	}
	.btn {
		display: flex;
		align-items: center;
		min-height: 44px;
		padding: 0 16px;
		border-radius: 12px;
		background: var(--text);
		color: var(--text-inverse);
		font-size: 15px;
		font-weight: 600;
	}
</style>
