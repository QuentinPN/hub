import type { ServiceStatus, ServiceSummary } from './api/types';

// Ce qui compte comme « en ligne » pour le compteur de la barre du haut ; « configuration
// requise » tourne, il ne manque que l'adresse.
const fine: ServiceStatus[] = ['online', 'config_required'];
// L'arrêt d'abord, puis le dégradé ; l'inconnu (agent muet) en dernier, ce n'est pas une panne.
const rank: Partial<Record<ServiceStatus, number>> = { down: 0, degraded: 1, unknown: 2 };

/** Les favoris qui ne sont pas en ligne, du plus grave au moins grave, ordre d'origine sinon. */
export function troubledFavorites(favorites: ServiceSummary[]): ServiceSummary[] {
	return favorites
		.filter((s) => !fine.includes(s.status))
		.sort((a, b) => (rank[a.status] ?? 3) - (rank[b.status] ?? 3));
}
