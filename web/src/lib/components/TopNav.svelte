<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';

	let { online, total }: { online?: number; total?: number } = $props();

	const tabs = [
		{
			label: 'Accueil',
			short: 'Accueil',
			href: '/',
			icon: 'M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z'
		},
		{
			label: 'Médias',
			short: 'Médias',
			href: '/medias',
			icon: 'M5 5h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zm5 4v6l5-3z'
		},
		{
			label: 'Téléchargements',
			short: 'Téléch.',
			href: '/telechargements',
			icon: 'M12 4v11m0 0-4-4m4 4 4-4M5 20h14'
		},
		{
			label: 'Infra',
			short: 'Infra',
			href: '/infra',
			icon: 'M5 4h14a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1zm0 10h14a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1v-4a1 1 0 0 1 1-1z'
		},
		{
			label: 'Tous les services',
			short: 'Services',
			href: '/services',
			icon: 'M4 6h16M4 12h16M4 18h10'
		}
	] as const;

	const path = $derived(page.url.pathname);
	const current = $derived(
		tabs.find((tab) => (tab.href === '/' ? path === '/' : path.startsWith(tab.href)))?.label ?? null
	);
	const onSettings = $derived(path.startsWith('/settings'));
</script>

<header class="top">
	<a class="brand" href={resolve('/')}>
		<!-- Le logo de Hub (static/favicon.svg), sans son carré de fond. -->
		<svg width="26" height="26" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"
			><rect x="3.5" y="3.5" width="7.5" height="7.5" rx="2" fill="var(--accent-play)"></rect><rect
				x="13"
				y="3.5"
				width="7.5"
				height="7.5"
				rx="2"
			></rect><rect x="3.5" y="13" width="7.5" height="7.5" rx="2"></rect><rect
				x="13"
				y="13"
				width="7.5"
				height="7.5"
				rx="2"
			></rect></svg
		>
		Hub
	</a>
	<nav aria-label="Sections">
		{#each tabs as tab (tab.label)}
			<a
				class="tab"
				class:on={current === tab.label}
				aria-current={current === tab.label ? 'page' : undefined}
				href={resolve(tab.href)}>{tab.label}</a
			>
		{/each}
	</nav>
	<div class="end">
		{#if path === '/'}
			<a class="custom" href={resolve('/personnaliser')}>Personnaliser</a>
		{/if}
		{#if total}
			<span class="pill tabular"
				><span class="ok" aria-hidden="true"></span>{online} sur {total} en ligne</span
			>
		{/if}
		<a
			class="gear"
			class:on={onSettings}
			href={resolve('/settings')}
			aria-label="Réglages"
			aria-current={onSettings ? 'page' : undefined}
			><svg
				width="18"
				height="18"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
				><!-- Un engrenage : l'ancien soleil se lisait comme un réglage clair / sombre. --><path
					d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"
				></path><circle cx="12" cy="12" r="3"></circle></svg
			></a
		>
	</div>
</header>

<nav class="bottom" aria-label="Navigation principale">
	{#each tabs as tab (tab.label)}
		{#snippet icon()}
			<svg
				width="22"
				height="22"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.8"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"><path d={tab.icon}></path></svg
			>
		{/snippet}
		<a
			class="btab"
			class:on={current === tab.label}
			aria-current={current === tab.label ? 'page' : undefined}
			href={resolve(tab.href)}>{@render icon()}{tab.short}</a
		>
	{/each}
</nav>

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 18px;
		font-weight: 700;
	}
	nav {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		padding: 5px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-pill);
		background: var(--surface-2);
		backdrop-filter: var(--blur);
		-webkit-backdrop-filter: var(--blur);
	}
	.tab {
		padding: 8px 18px;
		border-radius: var(--r-pill);
		color: var(--text-2);
		font-size: 15px;
		font-weight: 500;
	}
	a.tab:hover {
		color: var(--text);
	}
	.tab.on {
		background: rgba(255, 255, 255, 0.92);
		color: var(--text-inverse);
		opacity: 1;
	}
	.end {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.custom {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		font-size: 14px;
		font-weight: 600;
	}
	.gear {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-pill);
		background: var(--surface-2);
		color: var(--text);
	}
	.gear:hover {
		background: var(--surface-3);
	}
	.gear.on {
		border-color: transparent;
		background: rgba(255, 255, 255, 0.92);
		color: var(--text-inverse);
	}
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 10px 14px;
		border-radius: var(--r-pill);
		background: var(--surface-1);
		font-size: 14px;
	}
	.ok {
		width: 8px;
		height: 8px;
		border-radius: 99px;
		background: var(--status-online);
	}
	.bottom {
		display: none;
	}
	@media (max-width: 720px) {
		.top nav {
			display: none;
		}
		.bottom {
			position: fixed;
			z-index: 10;
			right: 12px;
			bottom: max(16px, env(safe-area-inset-bottom));
			left: 12px;
			display: flex;
			padding: 8px 6px;
			border: 1px solid rgba(255, 255, 255, 0.1);
			border-radius: 26px;
			background: rgba(28, 28, 32, 0.72);
			backdrop-filter: var(--blur);
			-webkit-backdrop-filter: var(--blur);
		}
		.btab {
			display: flex;
			flex: 1;
			flex-direction: column;
			align-items: center;
			justify-content: center;
			gap: 3px;
			min-height: 44px;
			color: var(--text-2);
			font-size: 11px;
			font-weight: 500;
		}
		.btab.on {
			color: var(--text);
			opacity: 1;
		}
		:global(body) {
			padding-bottom: 96px;
		}
	}
</style>
