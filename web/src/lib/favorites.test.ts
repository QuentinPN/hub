import { describe, expect, it } from 'vitest';
import type { ServiceStatus, ServiceSummary } from './api/types';
import { troubledFavorites } from './favorites';

const service = (slug: string, status: ServiceStatus): ServiceSummary => ({
	slug,
	name: slug,
	monogram: slug[0] ?? '',
	icon: null,
	color: '#000',
	category: 'Médias',
	hostId: 'debian-mini',
	status,
	latencyMs: null,
	note: '',
	favorite: true
});

describe('troubledFavorites', () => {
	it('ne garde rien quand tout tourne, adresse manquante comprise', () => {
		expect(
			troubledFavorites([service('emby', 'online'), service('dozzle', 'config_required')])
		).toEqual([]);
	});

	it("range l'arrêt avant le dégradé, et l'inconnu en dernier", () => {
		const list = [
			service('pihole', 'unknown'),
			service('emby', 'online'),
			service('jellyseerr', 'degraded'),
			service('sonarr', 'down'),
			service('radarr', 'degraded')
		];
		expect(troubledFavorites(list).map((s) => s.slug)).toEqual([
			'sonarr',
			'jellyseerr',
			'radarr',
			'pihole'
		]);
	});
});
