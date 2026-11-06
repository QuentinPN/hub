import type { ServiceStatus, ServiceSummary } from './api/types';

/** Cible de Ctrl+K, branché dans +layout.svelte. */
export const SEARCH_INPUT_ID = 'catalogue-search';

export type StatusFilter = 'all' | 'favorite' | ServiceStatus;
export type GroupBy = 'category' | 'host';

export interface CatalogueGroup {
	name: string;
	services: ServiceSummary[];
}

export interface CatalogueView {
	/** Services qui correspondent à la recherche, avant le filtre d'état : base des compteurs. */
	searched: ServiceSummary[];
	counts: Record<StatusFilter, number>;
	groups: CatalogueGroup[];
}

const normalize = (s: string) =>
	s
		.normalize('NFD')
		.replace(/\p{Diacritic}/gu, '')
		.toLowerCase();

export function matchesQuery(service: ServiceSummary, query: string): boolean {
	const needle = normalize(query.trim());
	if (!needle) return true;
	return [service.name, service.slug, service.hostId, service.category].some((v) =>
		normalize(v).includes(needle)
	);
}

// Les groupes suivent l'ordre d'apparition dans l'inventaire, pas l'ordre alphabétique :
// le serveur décide de l'ordre (plus tard, celui choisi par l'utilisateur).
export function catalogueView(
	services: ServiceSummary[],
	query: string,
	status: StatusFilter,
	groupBy: GroupBy
): CatalogueView {
	const searched = services.filter((s) => matchesQuery(s, query));
	const counts: Record<StatusFilter, number> = {
		all: searched.length,
		favorite: 0,
		online: 0,
		degraded: 0,
		down: 0,
		unknown: 0,
		config_required: 0
	};
	for (const s of searched) {
		counts[s.status]++;
		if (s.favorite) counts.favorite++;
	}

	const groups = new Map<string, ServiceSummary[]>();
	for (const s of searched) {
		if (status === 'favorite' ? !s.favorite : status !== 'all' && s.status !== status) continue;
		const key = groupBy === 'category' ? s.category : s.hostId;
		const list = groups.get(key);
		if (list) list.push(s);
		else groups.set(key, [s]);
	}
	return {
		searched,
		counts,
		groups: [...groups].map(([name, list]) => ({ name, services: list }))
	};
}
