import { describe, expect, it } from 'vitest';
import type { HistoryDay } from '#lib/api/types.ts';
import { dayTone, duration, eventView, groupByDay, summarize, uptime } from './history.ts';

const day = (d: Partial<HistoryDay>): HistoryDay => ({
	date: '2026-11-07',
	upMinutes: 0,
	downMinutes: 0,
	unknownMinutes: 0,
	latencyMs: null,
	outages: 0,
	...d
});

describe('dayTone', () => {
	it('distingue sans mesure, inconnu, sans panne, panne courte et longue', () => {
		expect(dayTone(day({}))).toBe('none');
		expect(dayTone(day({ unknownMinutes: 900, upMinutes: 540 }))).toBe('unknown');
		expect(dayTone(day({ upMinutes: 1440 }))).toBe('ok');
		expect(dayTone(day({ upMinutes: 1420, downMinutes: 20 }))).toBe('warn');
		expect(dayTone(day({ upMinutes: 1000, downMinutes: 440 }))).toBe('down');
	});
});

describe('uptime et duration', () => {
	it('écrit à la française, sans arrondir une panne à 100 %', () => {
		expect(uptime(0, 0)).toBeNull();
		expect(uptime(1440, 0)).toBe('100 %');
		expect(uptime(99_999, 1)).toBe('99,99 %');
		expect(duration(20)).toBe('20 min');
		expect(duration(72)).toBe('1 h 12');
		expect(duration(60 * 51)).toBe('2 j 3 h');
	});
	it('pondère la latence par les minutes en ligne', () => {
		const s = summarize([
			day({ date: '2026-11-06', upMinutes: 60, latencyMs: 10 }),
			day({ upMinutes: 180, latencyMs: 50, downMinutes: 5, outages: 1 })
		]);
		expect(s).toMatchObject({ latencyMs: 40, outages: 1, downMinutes: 5, firstDate: '2026-11-06' });
	});
});

describe('eventView', () => {
	const base = { id: 1, at: '2026-11-07T13:41:00Z' };
	it('dit chaque événement en français', () => {
		expect(
			eventView({ ...base, kind: 'outage', action: 'down', reason: 'adresse injoignable' })
		).toMatchObject({ title: 'En panne', detail: 'Adresse injoignable', badge: 'en cours' });
		expect(
			eventView({ ...base, kind: 'container', action: 'die', exitCode: 1, container: 'emby' })
		).toMatchObject({ tone: 'crash', title: 'Arrêt inattendu', detail: 'Conteneur emby · code 1' });
		expect(eventView({ ...base, kind: 'container', action: 'die', exitCode: 0 }).title).toBe(
			'Arrêté'
		);
		expect(
			eventView({ ...base, kind: 'version', action: 'changed', from: 'a/b:1', to: 'a/b:2' }).title
		).toBe('Nouvelle image');
		expect(
			eventView({ ...base, kind: 'edit', action: 'widget_connected', by: 'quentin' })
		).toMatchObject({ title: 'Widget connecté', detail: 'Par quentin' });
		expect(
			eventView({
				...base,
				kind: 'alert',
				action: 'sent',
				title: 'Emby est arrêté',
				via: ['digest']
			}).detail
		).toBe('« Emby est arrêté » · gardée pour le résumé de 08:00');
	});
	it('regroupe par jour local', () => {
		const now = new Date(2026, 10, 7, 15, 0);
		const g = groupByDay(
			[
				{ ...base, at: new Date(2026, 10, 7, 14, 0).toISOString(), kind: 'edit', action: 'x' },
				{ ...base, id: 2, at: new Date(2026, 10, 6, 9, 0).toISOString(), kind: 'edit', action: 'x' }
			],
			now
		);
		expect(g.map((x) => x.label)).toEqual(['Aujourd’hui', 'Hier, vendredi 6 novembre']);
	});
});
