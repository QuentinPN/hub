import { describe, expect, it } from 'vitest';
import { displayNameError, startPath, timeZoneChoices } from './general.ts';

describe('displayNameError', () => {
	it('accepte un nom ordinaire, accents et emoji compris', () => {
		expect(displayNameError('Hub de Quentin')).toBe('');
		expect(displayNameError('  Maison éclairée 🏠 ')).toBe('');
	});
	it('refuse le vide, le trop long et l’invisible, comme le serveur', () => {
		expect(displayNameError('   ')).not.toBe('');
		expect(displayNameError('é'.repeat(41))).not.toBe('');
		expect(displayNameError('é'.repeat(40))).toBe('');
		expect(displayNameError('Hub\u0007')).not.toBe('');
		expect(displayNameError('Hub\u202egnp.')).not.toBe('');
		expect(displayNameError('H\u200bub')).not.toBe('');
		expect(displayNameError('Hub\u00a0de Q')).not.toBe('');
		expect(displayNameError('Hub\tde Q')).not.toBe('');
	});
});

describe('startPath', () => {
	it('mène chaque page à sa route, l’inconnu à l’accueil', () => {
		expect(startPath('home')).toBe('/');
		expect(startPath('downloads')).toBe('/telechargements');
		expect(startPath(undefined)).toBe('/');
	});
});

describe('timeZoneChoices', () => {
	it('garde un fuseau enregistré que le navigateur ne connaît pas, sans doublon', () => {
		const list = timeZoneChoices('Europe/Paris', 'Etc/Ancien', 'Europe/Paris', '');
		expect(list).toContain('UTC');
		expect(list).toContain('Etc/Ancien');
		expect(list.filter((z) => z === 'Europe/Paris')).toHaveLength(1);
		expect(list).not.toContain('');
	});
});
