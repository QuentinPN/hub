import { getHome } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Promesse non attendue : la page affiche son squelette pendant le chargement.
export const load: PageLoad = ({ fetch }) => ({ home: getHome(fetch) });
