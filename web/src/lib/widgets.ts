import type { MediaItem, UpcomingItem, WidgetState } from './api/types';
import { formatDay } from './agents';

interface WidgetStateView {
	label: string;
	/** Ce qui se passe, en une phrase. */
	detail: string;
	color: string;
	/** Rouge : il faut agir. Jaune : Hub réessaie seul. */
	tone: 'ok' | 'warn' | 'bad' | 'idle';
}

const views: Record<WidgetState, WidgetStateView> = {
	ok: { label: 'Connecté', detail: '', color: 'var(--status-online)', tone: 'ok' },
	unreachable: {
		label: 'Injoignable',
		detail: 'La dernière lecture reste affichée, datée. Hub réessaie en espaçant.',
		color: 'var(--status-degraded)',
		tone: 'warn'
	},
	cert: {
		label: 'Certificat non reconnu',
		detail: 'Le service ne présente plus le certificat accepté. Vérifie son adresse.',
		color: 'var(--status-degraded)',
		tone: 'warn'
	},
	auth: {
		label: 'Clé refusée',
		detail: 'Hub ne réessaie plus tant que la clé n’a pas changé.',
		color: 'var(--status-down)',
		tone: 'bad'
	},
	unexpected: {
		label: 'Réponse inattendue',
		detail:
			'Le service répond, mais pas dans le format connu. Une mise à jour a pu le changer. Aucune donnée n’est devinée.',
		color: 'var(--status-down)',
		tone: 'bad'
	},
	stopped: {
		label: 'Service arrêté',
		detail: 'Ses conteneurs sont arrêtés, Hub n’interroge pas son API.',
		color: 'var(--status-unknown)',
		tone: 'idle'
	},
	address_changed: {
		label: 'Adresse changée',
		detail:
			'La clé a été testée sur une autre adresse : elle n’y part pas. Saisis-la de nouveau pour la tester ici.',
		color: 'var(--status-down)',
		tone: 'bad'
	}
};

export const widgetStateView = (state: WidgetState): WidgetStateView => views[state];

/** Ce que le test d'une clé répond, côté panneau de connexion. */
// Un compte peut être refusé avec le bon mot de passe : Beszel n'admet que les comptes vérifiés.
const accountRefused = {
	title: 'Compte refusé',
	detail:
		'Mot de passe faux, ou compte pas autorisé à se connecter : dans Beszel, il doit être vérifié (champ verified).'
};

/** Pour un compte, sa propre façon de dire le refus ; le reste ne change pas. */
export function accountStateView(state: WidgetState): WidgetStateView {
	const view = widgetStateView(state);
	if (state !== 'auth') return view;
	return {
		...view,
		label: accountRefused.title,
		detail: `${accountRefused.detail} Hub ne réessaie plus tant que le mot de passe n’a pas changé.`
	};
}

export function testFailure(
	result: WidgetState,
	account = false
): { title: string; detail: string } {
	if (account && result === 'auth') return accountRefused;
	switch (result) {
		case 'auth':
			return {
				title: 'Clé refusée',
				detail: 'Le service refuse cette clé. Rien n’est enregistré.'
			};
		case 'cert':
			return {
				title: 'Certificat non reconnu',
				detail: 'Accepte d’abord le certificat depuis l’adresse du service.'
			};
		case 'unexpected':
			return {
				title: 'Réponse inattendue',
				detail:
					'Le service répond, mais pas dans le format connu. Vérifie son adresse et sa version.'
			};
		default:
			return {
				title: 'Injoignable depuis le serveur',
				detail: 'Le service peut être filtré pour Hub tout en marchant pour toi.'
			};
	}
}

const decimal = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 });

/** « 12,4 Mo/s », comme le serveur l'écrit pour l'accueil. */
export function speedLabel(bytes: number): string {
	if (bytes >= 1_000_000) return `${decimal.format(bytes / 1_000_000)} Mo/s`;
	if (bytes >= 1_000) return `${Math.floor(bytes / 1_000)} Ko/s`;
	return `${bytes} o/s`;
}

const CONNECTIONS: Record<string, string> = {
	connected: 'connecté',
	firewalled: 'derrière un pare-feu',
	disconnected: 'déconnecté'
};

export const connectionLabel = (status: string) => CONNECTIONS[status] ?? status;

/** « 4 min », « 1 h 10 ». */
export function etaLabel(seconds: number): string {
	if (seconds <= 0) return '';
	const minutes = Math.ceil(seconds / 60);
	if (minutes < 60) return `${minutes} min`;
	return `${Math.floor(minutes / 60)} h ${String(minutes % 60).padStart(2, '0')}`;
}

const TORRENT_STATES: Record<string, string> = {
	downloading: 'Téléchargement',
	forcedDL: 'Téléchargement forcé',
	stalledDL: 'En attente de sources',
	metaDL: 'Récupération des métadonnées',
	queuedDL: 'En file',
	pausedDL: 'En pause',
	checkingDL: 'Vérification',
	allocating: 'Préparation du disque',
	moving: 'Déplacement',
	error: 'Erreur',
	missingFiles: 'Fichiers manquants'
};

export const torrentStateLabel = (state: string) => TORRENT_STATES[state] ?? state;

const QUEUE_STATES: Record<string, string> = {
	downloading: 'En cours',
	queued: 'En attente',
	delay: 'En attente',
	paused: 'En pause',
	completed: 'Terminé',
	failed: 'Échec',
	warning: 'À vérifier',
	downloadClientUnavailable: 'Client injoignable'
};

export const queueStateLabel = (status: string) => QUEUE_STATES[status] ?? status;

export function refreshLabel(seconds: number): string {
	if (seconds === 60) return 'Toutes les minutes';
	return seconds < 60 ? `Toutes les ${seconds} s` : `Toutes les ${seconds / 60} min`;
}

// Pas encore d'affiches réelles : un aplat stable, le même pour un même titre.
const TONES = ['#22303d', '#33293f', '#3a2a1c', '#1e3a33', '#3a2230', '#2a3347'];

function toneOf(title: string): string {
	let sum = 0;
	for (const c of title) sum = (sum + (c.codePointAt(0) ?? 0)) % TONES.length;
	return TONES[sum] ?? '#22303d';
}

const weekday = new Intl.DateTimeFormat('fr-FR', { weekday: 'short', day: 'numeric' });
const DAY_MS = 86_400_000;

/** « Aujourd’hui », « Demain », puis « lun. 9 », à l'heure de celui qui regarde. */
export function whenLabel(iso: string, now: Date): string {
	const at = new Date(iso);
	const start = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
	const days = Math.round((start(at) - start(now)) / DAY_MS);
	if (days === 0) return 'Aujourd’hui';
	if (days === 1) return 'Demain';
	return weekday.format(at);
}

/** Hub seul tient la clé : l'image passe par lui. */
function posterUrl(slug: string | undefined, poster: string | undefined): string | undefined {
	if (!slug || !poster) return undefined;
	return `/api/v1/posters/${encodeURIComponent(slug)}/${encodeURIComponent(poster)}`;
}

/** Ce qu'une connexion a lu reçoit sa date, sa teinte et son image ici ; le reste passe tel quel. */
export const upcomingPoster = (
	item: UpcomingItem,
	now: Date,
	slug?: string
): UpcomingItem & { image?: string } =>
	item.at
		? {
				...item,
				when: whenLabel(item.at, now),
				tone: item.tone || toneOf(item.title),
				image: posterUrl(item.source ?? slug, item.poster)
			}
		: item;

/** Un média ajouté, sous la forme des affiches de l'accueil. */
export const mediaPoster = (item: MediaItem, slug?: string): UpcomingItem & { image?: string } => ({
	title: item.title,
	detail: item.detail,
	when: item.addedAt ? formatDay(item.addedAt) : '',
	kind: item.kind,
	tone: toneOf(item.title),
	image: posterUrl(slug, item.poster)
});
