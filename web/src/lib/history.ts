import type { HistoryDay, HistoryEvent, HistoryKind } from '#lib/api/types.ts';

/** Filtres de la chronologie, dans l'ordre affiché. */
export const HISTORY_FILTERS: { id: HistoryKind; label: string; empty: string }[] = [
	{ id: 'outage', label: 'Pannes', empty: 'Aucune panne' },
	{ id: 'container', label: 'Conteneurs', empty: 'Aucun événement de conteneur' },
	{ id: 'version', label: 'Versions', empty: 'Aucun changement de version' },
	{ id: 'edit', label: 'Modifications', empty: 'Aucune modification' },
	{ id: 'alert', label: 'Alertes', empty: 'Aucune alerte envoyée' }
];

export type DayTone = 'ok' | 'warn' | 'down' | 'unknown' | 'none';

/** Vert sans panne, jaune au-dessus de 95 %, rouge en dessous ; hachuré si surtout inconnu. */
export function dayTone(d: HistoryDay): DayTone {
	const known = d.upMinutes + d.downMinutes;
	if (known + d.unknownMinutes === 0) return 'none';
	if (d.unknownMinutes > known) return 'unknown';
	if (d.downMinutes === 0) return 'ok';
	return d.upMinutes / known >= 0.95 ? 'warn' : 'down';
}

const percent = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 2 });

/** « 99,82 % » ; null sans minute connue. */
export function uptime(up: number, down: number): string | null {
	if (up + down === 0) return null;
	const p = (100 * up) / (up + down);
	// 99,999 ne s'arrondit pas à 100 : une panne reste visible.
	return `${percent.format(down > 0 ? Math.min(p, 99.99) : p)} %`;
}

/** « 20 min », « 1 h 12 », « 2 j 3 h ». */
export function duration(minutes: number): string {
	if (minutes < 60) return `${Math.max(1, Math.round(minutes))} min`;
	const h = Math.floor(minutes / 60);
	if (h < 24) {
		const m = Math.round(minutes % 60);
		return m ? `${h} h ${String(m).padStart(2, '0')}` : `${h} h`;
	}
	const d = Math.floor(h / 24);
	return h % 24 ? `${d} j ${h % 24} h` : `${d} j`;
}

export interface Summary {
	uptime: string | null;
	outages: number;
	downMinutes: number;
	/** Moyenne des jours mesurés, pondérée par leurs minutes en ligne. */
	latencyMs: number | null;
	/** Premier jour avec une mesure. */
	firstDate: string | null;
}

export function summarize(days: HistoryDay[]): Summary {
	let up = 0,
		down = 0,
		outages = 0,
		latSum = 0,
		latWeight = 0;
	let firstDate: string | null = null;
	for (const d of days) {
		up += d.upMinutes;
		down += d.downMinutes;
		outages += d.outages;
		if (d.latencyMs !== null && d.upMinutes > 0) {
			latSum += d.latencyMs * d.upMinutes;
			latWeight += d.upMinutes;
		}
		if (!firstDate && d.upMinutes + d.downMinutes + d.unknownMinutes > 0) firstDate = d.date;
	}
	return {
		uptime: uptime(up, down),
		outages,
		downMinutes: down,
		latencyMs: latWeight ? Math.round(latSum / latWeight) : null,
		firstDate
	};
}

/** Une date AAAA-MM-JJ du serveur, lue comme un jour du calendrier. */
const calendarDay = (date: string) => new Date(`${date}T12:00:00`);

export const shortDate = (date: string) =>
	calendarDay(date).toLocaleDateString('fr-FR', { day: 'numeric', month: 'short' });

export const longDate = (date: string) =>
	calendarDay(date).toLocaleDateString('fr-FR', { weekday: 'long', day: 'numeric', month: 'long' });

/** Ce que dit un jour de la barre, au survol ou au clavier. */
export function dayText(d: HistoryDay): string {
	const tone = dayTone(d);
	const parts = [longDate(d.date)];
	if (tone === 'none') parts.push('pas encore mesuré');
	else if (tone === 'unknown') parts.push('hôte injoignable, état inconnu');
	else {
		parts.push(`${uptime(d.upMinutes, d.downMinutes)} en ligne`);
		if (d.outages > 0) parts.push(`${d.outages} panne${d.outages > 1 ? 's' : ''}`);
		if (d.downMinutes > 0) parts.push(`${duration(d.downMinutes)} en panne`);
	}
	if (d.latencyMs !== null) parts.push(`latence ${d.latencyMs} ms`);
	return parts.join(' · ');
}

const EDITS: Record<string, string> = {
	service_updated: 'Fiche modifiée',
	service_reset: 'Personnalisation effacée',
	service_url_set: 'Adresse enregistrée',
	service_url_cleared: 'Adresse retirée',
	service_shown: 'Service affiché',
	widget_connected: 'Widget connecté',
	widget_disconnected: 'Widget retiré'
};

const VIA: Record<string, string> = {
	ntfy: 'ntfy',
	smtp: 'e-mail',
	digest: 'gardée pour le résumé de 08:00'
};

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

export type EventTone = 'down' | 'ok' | 'container' | 'crash' | 'version' | 'edit' | 'alert';

export interface EventView {
	tone: EventTone;
	title: string;
	detail: string;
	/** Durée d'une panne, ou « en cours ». */
	badge?: string;
}

export function eventView(e: HistoryEvent): EventView {
	const ctr = e.container ? `Conteneur ${e.container}` : 'Conteneur';
	switch (e.kind) {
		case 'outage':
			if (e.action === 'up') {
				return {
					tone: 'ok',
					title: 'Revenu en ligne',
					detail: e.minutes !== undefined ? `Après ${duration(e.minutes)} de panne` : 'Fin de panne'
				};
			}
			return {
				tone: 'down',
				title: 'En panne',
				detail: e.reason ? capitalize(e.reason) : 'Service injoignable',
				badge: e.minutes !== undefined ? duration(e.minutes) : 'en cours'
			};
		case 'container':
			switch (e.action) {
				case 'oom':
					return { tone: 'crash', title: 'Mémoire épuisée (OOM)', detail: ctr };
				case 'die':
					return e.exitCode
						? { tone: 'crash', title: 'Arrêt inattendu', detail: `${ctr} · code ${e.exitCode}` }
						: { tone: 'container', title: 'Arrêté', detail: ctr };
				case 'restart':
					return { tone: 'container', title: 'Redémarré', detail: ctr };
				default:
					return { tone: 'container', title: 'Démarré', detail: ctr };
			}
		case 'version': {
			const image = /[/:@]/.test(e.to ?? '');
			const change = `${e.from || '?'} → ${e.to || '?'}`;
			return {
				tone: 'version',
				title: image ? 'Nouvelle image' : 'Nouvelle version',
				detail: [change, !image && e.image ? `image ${e.image}` : '', e.container ?? '']
					.filter(Boolean)
					.join(' · ')
			};
		}
		case 'edit':
			return {
				tone: 'edit',
				title: EDITS[e.action] ?? 'Modifié dans Hub',
				detail: e.by ? `Par ${e.by}` : 'Dans Hub'
			};
		case 'alert': {
			const via = (e.via ?? []).map((v) => VIA[v] ?? v).join(' et ');
			return {
				tone: 'alert',
				title: e.action === 'recovery' ? 'Alerte de retour envoyée' : 'Alerte envoyée',
				detail: [e.title ? `« ${e.title} »` : '', via].filter(Boolean).join(' · ')
			};
		}
	}
}

const pad = (n: number) => String(n).padStart(2, '0');
const localDate = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

/** Titre d'un groupe de la chronologie, à l'heure de celui qui regarde. */
export function groupLabel(iso: string, now: Date): string {
	const at = new Date(iso);
	const day = localDate(at);
	const yesterday = new Date(now);
	yesterday.setDate(now.getDate() - 1);
	const long = at.toLocaleDateString('fr-FR', {
		weekday: 'long',
		day: 'numeric',
		month: 'long',
		year: at.getFullYear() === now.getFullYear() ? undefined : 'numeric'
	});
	if (day === localDate(now)) return 'Aujourd’hui';
	if (day === localDate(yesterday)) return `Hier, ${long}`;
	return capitalize(long);
}

export const timeOf = (iso: string) =>
	new Date(iso).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });

/** Les événements regroupés par jour, dans l'ordre reçu (le plus récent d'abord). */
export function groupByDay(events: HistoryEvent[], now: Date) {
	const groups: { label: string; events: HistoryEvent[] }[] = [];
	for (const e of events) {
		const label = groupLabel(e.at, now);
		const last = groups.at(-1);
		if (last?.label === label) last.events.push(e);
		else groups.push({ label, events: [e] });
	}
	return groups;
}

/** « 7 novembre 2026 à 15:20 ». */
export const startText = (iso: string) => {
	const d = new Date(iso);
	return `${d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' })} à ${timeOf(iso)}`;
};
