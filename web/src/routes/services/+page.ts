import { getHiddenGroups, getServices } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	await parent();
	// Liste des masqués : facultative, son absence ne doit pas empêcher le catalogue.
	return { services: getServices(fetch), hidden: getHiddenGroups(fetch).catch(() => []) };
};
