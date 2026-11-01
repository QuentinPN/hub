import { getHome } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Promesse non attendue : la page affiche son squelette pendant le chargement. On attend
// d'abord la garde d'auth du layout : sans session, la requête échouerait en 401 sans
// personne pour traiter le rejet.
export const load: PageLoad = async ({ fetch, parent }) => {
	await parent();
	return { home: getHome(fetch) };
};
