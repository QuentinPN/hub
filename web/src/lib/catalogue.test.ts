import { describe, expect, it } from 'vitest';
import type { ServiceSummary } from './api/types';
import { catalogueView, matchesQuery } from './catalogue';

const svc = (
	name: string,
	category: string,
	hostId: string,
	status: ServiceSummary['status']
): ServiceSummary => ({
	slug: name.toLowerCase(),
	name,
	monogram: name[0] ?? '?',
	color: '#333',
	category,
	hostId,
	status,
	latencyMs: null,
	note: '',
	favorite: false
});

const emby = svc('Emby', 'Médias', 'debian-mini', 'online');
const qbit = svc('qBittorrent', 'Téléchargements', 'qnap-ts464', 'online');
const pihole = svc('Pi-hole', 'Réseau', 'rpi5', 'unknown');
const inventory = [
	emby,
	svc('Prowlarr', 'Médias', 'debian-mini', 'config_required'),
	qbit,
	pihole,
	svc('Jellyseerr', 'Médias', 'debian-mini', 'degraded')
];

describe('matchesQuery', () => {
	it('ignore la casse et les accents', () => {
		expect(matchesQuery(emby, 'MEDIAS')).toBe(true);
		expect(matchesQuery(qbit, 'telech')).toBe(true);
	});

	it("cherche aussi dans l'hôte", () => {
		expect(matchesQuery(pihole, 'rpi')).toBe(true);
		expect(matchesQuery(emby, 'rpi')).toBe(false);
	});
});

describe('catalogueView', () => {
	it("compte les états sur la recherche, pas sur le filtre d'état", () => {
		const v = catalogueView(inventory, 'debian', 'online', 'category');
		expect(v.counts).toMatchObject({ all: 3, online: 1, config_required: 1, degraded: 1 });
		expect(v.groups).toEqual([{ name: 'Médias', services: [emby] }]);
	});

	it("groupe par hôte dans l'ordre d'apparition", () => {
		const v = catalogueView(inventory, '', 'all', 'host');
		expect(v.groups.map((g) => g.name)).toEqual(['debian-mini', 'qnap-ts464', 'rpi5']);
		expect(v.groups[0]?.services).toHaveLength(3);
	});

	it('ne renvoie aucun groupe quand rien ne correspond', () => {
		expect(catalogueView(inventory, 'zzz', 'all', 'category').groups).toEqual([]);
	});
});
