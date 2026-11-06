import { describe, expect, it } from 'vitest';
import {
	connectionLabel,
	mediaPoster,
	speedLabel,
	refreshLabel,
	testFailure,
	upcomingPoster,
	whenLabel,
	widgetStateView
} from './widgets';

describe('widgets', () => {
	it('dit quand il faut agir et quand Hub réessaie seul', () => {
		expect(widgetStateView('auth').tone).toBe('bad');
		expect(widgetStateView('unreachable').tone).toBe('warn');
		expect(widgetStateView('stopped').tone).toBe('idle');
		expect(widgetStateView('ok').detail).toBe('');
	});

	it('explique un test de clé raté', () => {
		expect(testFailure('auth').title).toBe('Clé refusée');
		expect(testFailure('unreachable').title).toBe('Injoignable depuis le serveur');
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
