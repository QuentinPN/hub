import { describe, expect, it } from 'vitest';
import {
	accountStateView,
	connectionLabel,
	etaLabel,
	mediaPoster,
	posterUrl,
	queueStateLabel,
	speedLabel,
	torrentStateLabel,
	refreshLabel,
	testFailure,
	upcomingPoster,
	whenLabel,
	widgetStateView
} from './widgets';

describe('widgets', () => {
	it('demande une image à Hub, jamais au service', () => {
		expect(posterUrl('emby', 'serie.backdrop')).toBe('/api/v1/posters/emby/serie.backdrop');
		expect(posterUrl('emby', undefined)).toBeUndefined();
		expect(posterUrl(undefined, 'serie')).toBeUndefined();
	});

	it('dit quand il faut agir et quand Hub réessaie seul', () => {
		expect(widgetStateView('auth').tone).toBe('bad');
		expect(widgetStateView('unreachable').tone).toBe('warn');
		expect(widgetStateView('stopped').tone).toBe('idle');
		expect(widgetStateView('ok').detail).toBe('');
	});

	it('explique un test de clé raté', () => {
		expect(testFailure('auth').title).toBe('Clé refusée');
		expect(testFailure('unreachable').title).toBe('Injoignable depuis le serveur');
		expect(testFailure('auth', true).title).toBe('Compte refusé');
		expect(testFailure('auth', true).detail).toContain('vérifié');
		expect(testFailure('unreachable', true).title).toBe('Injoignable depuis le serveur');
		expect(accountStateView('auth').label).toBe('Compte refusé');
		expect(accountStateView('ok').label).toBe('Connecté');
	});

	it('écrit la date à venir et garde ce qui n’a pas de date', () => {
		const now = new Date(2030, 0, 7, 22, 0);
		expect(whenLabel(new Date(2030, 0, 7, 23, 0).toISOString(), now)).toBe('Aujourd’hui');
		expect(whenLabel(new Date(2030, 0, 8, 1, 0).toISOString(), now)).toBe('Demain');
		expect(whenLabel(new Date(2030, 0, 14, 1, 0).toISOString(), now)).toBe('lun. 14');
		const old = { title: 'X', detail: '', when: 'Lun', kind: 'movie' as const, tone: '#000' };
		expect(upcomingPoster(old, now)).toEqual(old);
		const read = { ...old, when: '', tone: '', at: new Date(2030, 0, 8, 1, 0).toISOString() };
		expect(upcomingPoster(read, now).when).toBe('Demain');
		expect(upcomingPoster(read, now).tone).not.toBe('');
	});

	it('écrit les vitesses et l’état de la connexion', () => {
		expect(speedLabel(12_400_000)).toBe('12,4 Mo/s');
		expect(speedLabel(850_000)).toBe('850 Ko/s');
		expect(speedLabel(0)).toBe('0 o/s');
		expect(connectionLabel('firewalled')).toBe('derrière un pare-feu');
	});

	it('écrit le temps restant et les états', () => {
		expect(etaLabel(240)).toBe('4 min');
		expect(etaLabel(4200)).toBe('1 h 10');
		expect(etaLabel(0)).toBe('');
		expect(torrentStateLabel('stalledDL')).toBe('En attente de sources');
		expect(torrentStateLabel('nouveau')).toBe('nouveau');
		expect(queueStateLabel('queued')).toBe('En attente');
	});

	it('fait passer les images par Hub', () => {
		const now = new Date(2030, 0, 7);
		const read = {
			title: 'X',
			detail: '',
			when: '',
			kind: 'movie' as const,
			tone: '',
			at: new Date(2030, 0, 8).toISOString(),
			poster: '12',
			source: 'radarr'
		};
		expect(upcomingPoster(read, now).image).toBe('/api/v1/posters/radarr/12');
		expect(upcomingPoster({ ...read, source: undefined }, now, 'sonarr').image).toBe(
			'/api/v1/posters/sonarr/12'
		);
		expect(upcomingPoster({ ...read, poster: undefined }, now).image).toBeUndefined();
		const added = { title: 'X', detail: '', kind: 'movie' as const, addedAt: '', poster: 'a/b' };
		expect(mediaPoster(added, 'emby').image).toBe('/api/v1/posters/emby/a%2Fb');
		expect(mediaPoster(added).image).toBeUndefined();
	});

	it('écrit la fréquence', () => {
		expect(refreshLabel(30)).toBe('Toutes les 30 s');
		expect(refreshLabel(60)).toBe('Toutes les minutes');
		expect(refreshLabel(300)).toBe('Toutes les 5 min');
	});

	it('donne la même teinte au même titre', () => {
		const item = {
			title: 'Un film',
			detail: 'Film',
			kind: 'movie' as const,
			addedAt: '2026-11-05T10:00:00Z'
		};
		expect(mediaPoster(item).tone).toBe(mediaPoster({ ...item, detail: 'Autre' }).tone);
		expect(mediaPoster(item).when).toContain('nov');
		expect(mediaPoster({ ...item, addedAt: '' }).when).toBe('');
	});
});
