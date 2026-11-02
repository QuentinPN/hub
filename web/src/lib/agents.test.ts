import { describe, expect, it } from 'vitest';
import {
	agentStateView,
	ago,
	countdown,
	hasAgent,
	hostIdFrom,
	minutesLeft,
	renewalDay,
	validHostId
} from './agents';

const now = Date.parse('2026-11-02T18:00:00Z');
const before = (s: number) => new Date(now - s * 1000).toISOString();
// Intl sépare le nombre et l'unité par une espace insécable fine : on compare le texte seul.
const plain = (s: string) => s.replace(/\s/g, ' ');

describe('ago', () => {
	it('choisit l’unité la plus lisible', () => {
		expect(plain(ago(before(8), now))).toBe('il y a 8 s');
		expect(plain(ago(before(3 * 60 + 20), now))).toBe('il y a 3 min');
		expect(plain(ago(before(2 * 3600), now))).toBe('il y a 2 h');
		expect(plain(ago(before(3 * 86400), now))).toBe('il y a 3 j');
	});
	it('ne part pas dans le futur si les horloges diffèrent', () => {
		expect(plain(ago(before(-5), now))).toBe('il y a 0 s');
	});
});

describe('compte à rebours', () => {
	it('affiche minutes et secondes, puis s’arrête à zéro', () => {
		expect(countdown(before(-(14 * 60 + 32)), now)).toBe('14:32');
		expect(countdown(before(-5), now)).toBe('0:05');
		expect(countdown(before(10), now)).toBe('0:00');
		expect(minutesLeft(before(-(10 * 60 + 1)), now)).toBe(11);
	});
});

it('annonce le renouvellement dix jours avant la fin du certificat', () => {
	expect(renewalDay('2026-12-02T12:00:00Z')).toBe('22 nov.');
});

describe('identifiant d’hôte', () => {
	it('se déduit du nom affiché', () => {
		expect(hostIdFrom('mini-PC Geekom')).toBe('mini-pc-geekom');
		expect(hostIdFrom('  Évier  du NAS ! ')).toBe('evier-du-nas');
		expect(hostIdFrom('x'.repeat(80))).toHaveLength(63);
	});
	it('suit la règle du serveur', () => {
		expect(validHostId('hub-homeserver')).toBe(true);
		expect(validHostId('-nas')).toBe(false);
		expect(validHostId('NAS')).toBe(false);
		expect(validHostId('')).toBe(false);
		expect(validHostId('a'.repeat(64))).toBe(false);
	});
});

it('ne montre jamais un agent injoignable en rouge', () => {
	expect(agentStateView('offline').color).toBe('var(--status-unknown)');
	expect(agentStateView('offline').hatched).toBe(true);
	expect(hasAgent('offline')).toBe(true);
	expect(hasAgent('pending')).toBe(false);
	expect(hasAgent('revoked')).toBe(false);
});
