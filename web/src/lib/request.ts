import type {
	IssueType,
	PendingRequest,
	RequestSearchItem,
	RequestStats,
	RequestStatus,
	SearchStatus,
	UpcomingItem,
	WidgetRow
} from './api/types';
import { etaLabel, posterUrl, toneOf } from './widgets';

/** Le connecteur Request : c'est lui qu'on reconnaît, jamais le nom du service. */
export const REQUEST_KIND = 'request';

/** Mêmes bornes que le serveur : un motif de refus de 1 à 500 caractères, une recherche de 2 à 100. */
export const REASON_MAX = 500;
export const SEARCH_MIN = 2;
export const SEARCH_MAX = 100;
export const SEARCH_DEBOUNCE_MS = 300;

/** Un seul vocabulaire pour les demandes et pour ce que la recherche dit d'un titre. */
const STATUS_LABELS: Record<RequestStatus | SearchStatus, string> = {
	pending: 'En attente',
	approved: 'Approuvée',
	declined: 'Refusée',
	cancelled: 'Annulée',
	sent: 'Envoyée',
	failed: 'Échec',
	available: 'Disponible',
	partial: 'Partiel',
	downloading: 'En cours',
	requested: 'Demandé'
};

export const statusLabel = (status: string): string =>
	(STATUS_LABELS as Record<string, string>)[status] ?? status;

export type StatusTone = 'ok' | 'bad' | 'accent' | 'neutral';

const STATUS_TONES: Record<string, StatusTone> = {
	available: 'ok',
	failed: 'bad',
	declined: 'bad',
	approved: 'accent',
	sent: 'accent',
	downloading: 'accent',
	partial: 'accent',
	requested: 'accent'
};

export const statusTone = (status: string): StatusTone => STATUS_TONES[status] ?? 'neutral';

const ISSUE_LABELS: Record<IssueType, string> = {
	video: 'Vidéo',
	audio: 'Audio',
	subtitles: 'Sous-titres',
	other: 'Autre'
};

export const issueTypeLabel = (type: string): string =>
	(ISSUE_LABELS as Record<string, string>)[type] ?? type;

/** « S1E3 », « S2 », rien pour un film. */
export function episodeLabel(season?: number, episode?: number): string {
	if (season === undefined && episode === undefined) return '';
	return `${season === undefined ? '' : `S${season}`}${episode === undefined ? '' : `E${episode}`}`;
}

const DAY_S = 86_400;

/** « 12 min », « 1 h 10 », « 3 j 4 h » ; « — » quand le serveur n'a pas de médiane. */
export function durationLabel(seconds: number | null): string {
	if (seconds === null) return '—';
	const s = Math.round(seconds / 60) * 60;
	if (s < 60) return '< 1 min';
	if (s < DAY_S) return etaLabel(s);
	const days = Math.floor(s / DAY_S);
	const hours = Math.round((s % DAY_S) / 3600);
	if (hours === 0) return `${days} j`;
	return hours === 24 ? `${days + 1} j` : `${days} j ${hours} h`;
}

/** Le motif de refus tel qu'il part, ou null s'il est vide ou trop long. */
export function cleanReason(raw: string): string | null {
	const reason = raw.trim();
	const length = [...reason].length;
	return length >= 1 && length <= REASON_MAX ? reason : null;
}

/** Ce qui part à la recherche, ou null : en dessous de 2 caractères, on ne demande rien. */
export function searchTerm(raw: string): string | null {
	const term = raw.trim();
	const length = [...term].length;
	return length >= SEARCH_MIN && length <= SEARCH_MAX ? term : null;
}

/** La recherche n'existe que pour une connexion de ce connecteur, établie et encore prise en charge. */
export const requestSlug = (rows: WidgetRow[]): string | undefined =>
	rows.find(
		(r) =>
			r.kind === REQUEST_KIND &&
			r.support === 'ready' &&
			r.state !== '' &&
			r.state !== 'unsupported'
	)?.slug;

/** « Demander » n'est proposé que pour un titre qu'Request ne connaît pas encore. */
export const isRequestable = (item: RequestSearchItem) => item.status === null;

export const searchKey = (item: Pick<RequestSearchItem, 'mediaType' | 'tmdbId'>) =>
	`${item.mediaType}:${item.tmdbId}`;

const mediaKind = (type: 'movie' | 'tv') => (type === 'movie' ? 'Film' : 'Série');

/** Une demande à approuver, sous la forme des affiches. */
export function pendingPoster(
	item: PendingRequest,
	slug: string
): UpcomingItem & { image?: string } {
	const seasons = item.seasons.length > 0 ? `S${item.seasons.join(', S')}` : '';
	const extra = item.mediaType === 'movie' ? (item.year ?? '') : seasons;
	return {
		title: item.title,
		detail: `${item.requestedBy}${item.is4k ? ' · 4K' : ''}`,
		when: [mediaKind(item.mediaType), extra].filter(Boolean).join(' · '),
		kind: item.mediaType === 'movie' ? 'movie' : 'episode',
		tone: toneOf(item.title),
		image: posterUrl(slug, item.poster)
	};
}

/** Un titre trouvé par la recherche, sous la forme des affiches. */
export function searchPoster(
	item: RequestSearchItem,
	slug: string
): UpcomingItem & { image?: string } {
	return {
		title: item.title,
		detail: item.year ? String(item.year) : '',
		when: mediaKind(item.mediaType),
		kind: item.mediaType === 'movie' ? 'movie' : 'episode',
		tone: toneOf(item.title),
		image: posterUrl(slug, item.poster)
	};
}

export interface SparklineShape {
	/** Le tracé, vide sans mesure. */
	line: string;
	/** Le même tracé fermé sur la base, pour le remplissage. */
	area: string;
	end: { x: number; y: number } | null;
}

const EMPTY_SHAPE: SparklineShape = { line: '', area: '', end: null };
const round1 = (n: number) => String(Math.round(n * 10) / 10);

/** Courbe d'une série dans `width` × `height`, bornée à son maximum ; une série nulle reste à plat. */
export function sparkline(
	values: number[],
	width: number,
	height: number,
	pad = 4
): SparklineShape {
	const only = values[0];
	if (only === undefined) return EMPTY_SHAPE;
	const max = Math.max(...values, 0);
	const innerW = width - 2 * pad;
	const innerH = height - 2 * pad;
	const base = pad + innerH;
	const y = (v: number) => (max === 0 ? base : pad + innerH * (1 - v / max));
	// Un seul point ne trace rien : il devient un trait à plat.
	const points =
		values.length === 1
			? [
					{ x: pad, y: y(only) },
					{ x: width - pad, y: y(only) }
				]
			: values.map((v, i) => ({ x: pad + (innerW * i) / (values.length - 1), y: y(v) }));
	const line = points
		.map((p, i) => `${i === 0 ? 'M' : 'L'}${round1(p.x)} ${round1(p.y)}`)
		.join(' ');
	const first = points[0];
	const last = points[points.length - 1];
	if (!first || !last) return EMPTY_SHAPE;
	return {
		line,
		area: `${line} L${round1(last.x)} ${round1(base)} L${round1(first.x)} ${round1(base)} Z`,
		end: last
	};
}

const monthYear = new Intl.DateTimeFormat('fr-FR', {
	month: 'long',
	year: 'numeric',
	timeZone: 'UTC'
});
const plural = (n: number, one: string, many: string) => `${n} ${n > 1 ? many : one}`;

/** Ce que la courbe dit, pour qui ne la voit pas. */
export function sparklineSummary(byMonth: RequestStats['byMonth']): string {
	const first = byMonth[0];
	if (!first) return 'Aucune demande sur la période.';
	const total = byMonth.reduce((n, m) => n + m.count, 0);
	const top = byMonth.reduce((a, m) => (m.count > a.count ? m : a), first);
	const head = `${plural(total, 'demande', 'demandes')} sur ${byMonth.length} mois`;
	if (top.count === 0) return `${head}.`;
	return `${head}, le plus actif : ${monthYear.format(new Date(`${top.month}-01T00:00:00Z`))} avec ${top.count}.`;
}

/** Part des demandes en 4K, en pourcentage entier ; null sans aucune demande. */
export function fourKShare(quality: RequestStats['byQuality']): number | null {
	const total = quality.standard + quality.fourK;
	return total === 0 ? null : Math.round((quality.fourK / total) * 100);
}
