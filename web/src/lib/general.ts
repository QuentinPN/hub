import type { HistoryDays, StartPage, WidgetRefresh } from '#lib/api/types.ts';

type StartPath = '/' | '/medias' | '/telechargements' | '/infra';

/** Pages où Hub peut s'ouvrir, dans l'ordre de la barre du haut. */
export const START_PAGES: { value: StartPage; label: string; path: StartPath }[] = [
	{ value: 'home', label: 'Accueil', path: '/' },
	{ value: 'medias', label: 'Médias', path: '/medias' },
	{ value: 'downloads', label: 'Téléchargements', path: '/telechargements' },
	{ value: 'infra', label: 'Infra', path: '/infra' }
];

export const startPath = (page: StartPage | undefined): StartPath =>
	START_PAGES.find((p) => p.value === page)?.path ?? '/';

export const REFRESH_MODES: { value: WidgetRefresh; label: string }[] = [
	{ value: 'normal', label: 'Normal (30 s, temps réel quand possible)' },
	{ value: 'economy', label: 'Économe (2 min)' }
];

export const HISTORY_RETENTIONS: { value: HistoryDays; label: string }[] = [
	{ value: 30, label: '30 jours' },
	{ value: 90, label: '90 jours' },
	{ value: 180, label: '180 jours' },
	{ value: 365, label: '1 an' }
];

/** Même règle que le serveur, pour le dire avant d'envoyer. */
export const DISPLAY_NAME_MAX = 40;

export function displayNameError(name: string): string {
	const trimmed = name.trim();
	if (trimmed === '') return 'Le nom ne peut pas être vide.';
	if ([...trimmed].length > DISPLAY_NAME_MAX) return `${DISPLAY_NAME_MAX} caractères au plus.`;
	// Contrôles, marques de direction et caractères invisibles : ils déguiseraient l'onglet.
	if (/[\p{Cc}\p{Cf}\p{Co}\p{Cn}\p{Zl}\p{Zp}]/u.test(trimmed) || /[^\S ]/u.test(trimmed))
		return 'Caractère non autorisé.';
	return '';
}

/**
 * Fuseaux proposés : ceux que connaît le navigateur, plus ceux déjà en jeu (le choix
 * enregistré, celui du serveur) s'il ne les connaît pas. Le serveur vérifie chacun.
 */
export function timeZoneChoices(...known: string[]): string[] {
	const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
	const list = new Set(intl.supportedValuesOf?.('timeZone') ?? []);
	list.add('UTC');
	for (const z of known) if (z) list.add(z);
	return [...list].sort((a, b) => a.localeCompare(b));
}

export function browserTimeZone(): string {
	return Intl.DateTimeFormat().resolvedOptions().timeZone ?? '';
}
