import type { StartPage, WidgetRefresh } from '#lib/api/types.ts';

/** État partagé entre les pages et la barre du haut, rendue une seule fois par le layout. */
export const shell = $state<{
	summary: { online: number; total: number } | null;
	/** Réglages « Général » utiles à toutes les pages, lus avec l'état de connexion. */
	displayName: string;
	startPage: StartPage;
	widgetRefresh: WidgetRefresh;
}>({
	summary: null,
	displayName: 'Hub',
	startPage: 'home',
	widgetRefresh: 'normal'
});

/** Titre d'onglet : la page, puis le nom affiché. */
export const tabTitle = (page: string) => `${page} · ${shell.displayName}`;

/** Relecture des widgets par les pages : 30 s, ou 2 min en « Économe ». */
export const widgetPollMs = () => (shell.widgetRefresh === 'economy' ? 120_000 : 30_000);
