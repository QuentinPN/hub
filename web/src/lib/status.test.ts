import { describe, expect, it } from 'vitest';
import { statusStyle } from './status';

describe('statusStyle', () => {
	it("n'affiche jamais un état inconnu avec la couleur d'une panne", () => {
		expect(statusStyle('unknown').color).not.toBe(statusStyle('down').color);
		expect(statusStyle('unknown').label).toBe('Inconnu');
	});

	it('distingue « configuration requise » de « arrêté »', () => {
		expect(statusStyle('config_required').label).toBe('Configuration requise');
	});
});
