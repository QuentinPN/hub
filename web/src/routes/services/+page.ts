import { getHiddenGroups, getServices, getWidgetRows } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	await parent();
	// Liste des masqués : facultative, son absence ne doit pas empêcher le catalogue.
	// Les connexions : facultatives aussi, elles disent seulement si Request peut répondre à la recherche.
	return {
		services: getServices(fetch),
		hidden: getHiddenGroups(fetch).catch(() => []),
		widgets: getWidgetRows(fetch).catch(() => [])
	};
};
