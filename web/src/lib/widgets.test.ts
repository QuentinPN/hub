import { describe, expect, it } from 'vitest';
import { mediaPoster, refreshLabel, testFailure, widgetStateView } from './widgets';

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
