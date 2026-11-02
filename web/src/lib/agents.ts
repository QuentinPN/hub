import type { AgentState } from './api/types';

interface StateView {
	label: string;
	/** Couleur de la pastille : même vocabulaire que les services, plus l'accent « en attente ». */
	color: string;
	/** Hachuré comme un service inconnu : l'hôte n'est pas en panne, on ne sait plus. */
	hatched: boolean;
}

const views: Record<AgentState, StateView> = {
	connected: { label: 'Connecté', color: 'var(--status-online)', hatched: false },
	docker_down: { label: 'Docker injoignable', color: 'var(--status-degraded)', hatched: false },
	offline: { label: 'Injoignable', color: 'var(--status-unknown)', hatched: true },
	pending: { label: 'En attente d’enrôlement', color: 'var(--accent-play)', hatched: false },
	expired: { label: 'Certificat expiré', color: 'var(--status-degraded)', hatched: false },
	revoked: { label: 'Révoqué', color: 'var(--status-unknown)', hatched: false },
	none: { label: 'Aucun agent', color: 'var(--status-unknown)', hatched: false }
};

export const agentStateView = (state: AgentState): StateView => views[state];

/** Un agent a un certificat qui fonctionne : ses conteneurs ont un sens et il peut être révoqué. */
export const hasAgent = (state: AgentState) =>
	state === 'connected' || state === 'docker_down' || state === 'offline';

const relative = new Intl.RelativeTimeFormat('fr', { style: 'short', numeric: 'always' });
const steps: [Intl.RelativeTimeFormatUnit, number][] = [
	['second', 60],
	['minute', 60],
	['hour', 24],
	['day', Infinity]
];

/** « il y a 8 s », « il y a 3 min »… */
export function ago(iso: string, now: number): string {
	let value = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
	for (const [unit, size] of steps) {
		if (value < size) return relative.format(-value, unit);
		value = Math.floor(value / size);
	}
	return relative.format(-value, 'day');
}

/** Temps restant en « mm:ss », jamais négatif. */
export function countdown(iso: string, now: number): string {
	const s = Math.max(0, Math.floor((Date.parse(iso) - now) / 1000));
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** « dans 11 min » pour un code qui expire. */
export const minutesLeft = (iso: string, now: number) =>
	Math.max(0, Math.ceil((Date.parse(iso) - now) / 60_000));

const day = new Intl.DateTimeFormat('fr-FR', { day: 'numeric', month: 'short' });
export const formatDay = (iso: string) => day.format(new Date(iso));

// L'agent renouvelle aux deux tiers de ses 30 jours (AGENT-STUDY.md) : 10 jours avant la fin.
const RENEW_BEFORE_END = 10 * 24 * 3600 * 1000;
export const renewalDay = (certExpiresAt: string) =>
	formatDay(new Date(Date.parse(certExpiresAt) - RENEW_BEFORE_END).toISOString());

/** Même règle que le serveur (pki.ValidHostID) : gravé dans le certificat, il ne change plus. */
export const HOST_ID_PATTERN = '^[a-z0-9][a-z0-9-]{0,62}$';
export const validHostId = (id: string) => new RegExp(HOST_ID_PATTERN).test(id);

/** Identifiant proposé à partir du nom affiché : « mini-PC Geekom » → « mini-pc-geekom ». */
export function hostIdFrom(name: string): string {
	return name
		.normalize('NFD')
		.replace(/[̀-ͯ]/g, '')
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+/, '')
		.slice(0, 63)
		.replace(/-+$/, '');
}
