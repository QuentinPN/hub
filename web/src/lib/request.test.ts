import { describe, expect, it } from 'vitest';
import type { PendingRequest, RequestSearchItem, WidgetRow } from './api/types';
import {
	cleanReason,
	durationLabel,
	episodeLabel,
	fourKShare,
	isRequestable,
	issueTypeLabel,
	pendingPoster,
	searchPoster,
	searchTerm,
	sparkline,
	sparklineSummary,
	statusLabel,
	statusTone,
	requestSlug
} from './request';

describe('libellés', () => {
	it('dit chaque statut de demande en français', () => {
		expect(
			(
				['pending', 'approved', 'declined', 'cancelled', 'sent', 'failed', 'available'] as const
			).map(statusLabel)
		).toEqual(['En attente', 'Approuvée', 'Refusée', 'Annulée', 'Envoyée', 'Échec', 'Disponible']);
	});

	it('dit aussi ce que la recherche sait d’un titre, et garde l’inconnu tel quel', () => {
		expect(statusLabel('downloading')).toBe('En cours');
		expect(statusLabel('requested')).toBe('Demandé');
		expect(statusLabel('nouveau')).toBe('nouveau');
	});

	it('signale l’échec et le refus, rassure sur la disponibilité', () => {
		expect(statusTone('failed')).toBe('bad');
		expect(statusTone('declined')).toBe('bad');
		expect(statusTone('available')).toBe('ok');
		expect(statusTone('pending')).toBe('neutral');
	});

	it('nomme les signalements et leur épisode', () => {
		expect(['video', 'audio', 'subtitles', 'other'].map(issueTypeLabel)).toEqual([
			'Vidéo',
			'Audio',
			'Sous-titres',
			'Autre'
		]);
		expect(episodeLabel(1, 3)).toBe('S1E3');
		expect(episodeLabel(2)).toBe('S2');
		expect(episodeLabel()).toBe('');
	});
});

describe('durationLabel', () => {
	it('écrit des durées lisibles, jusqu’aux jours', () => {
		expect(durationLabel(null)).toBe('—');
		expect(durationLabel(20)).toBe('< 1 min');
		expect(durationLabel(720)).toBe('12 min');
		expect(durationLabel(4200)).toBe('1 h 10');
		expect(durationLabel(93_600)).toBe('1 j 2 h');
		expect(durationLabel(3 * 86_400)).toBe('3 j');
	});

	it('ne lit jamais « 24 h 00 »', () => {
		expect(durationLabel(86_399)).toBe('1 j');
		expect(durationLabel(86_400 + 23.6 * 3600)).toBe('2 j');
	});
});

describe('saisies', () => {
	it('garde un motif de 1 à 500 caractères, sans les blancs autour', () => {
		expect(cleanReason('  Déjà là  ')).toBe('Déjà là');
		expect(cleanReason('   ')).toBeNull();
		expect(cleanReason('a'.repeat(500))).toBe('a'.repeat(500));
		expect(cleanReason('a'.repeat(501))).toBeNull();
		expect(cleanReason('😀'.repeat(500))).toBe('😀'.repeat(500));
		expect(cleanReason('😀'.repeat(501))).toBeNull();
	});

	it('ne cherche qu’à partir de 2 caractères utiles', () => {
		expect(searchTerm('m')).toBeNull();
		expect(searchTerm('  m ')).toBeNull();
		expect(searchTerm(' ma ')).toBe('ma');
		expect(searchTerm('a'.repeat(101))).toBeNull();
		expect(searchTerm('😀')).toBeNull();
		expect(searchTerm('😀😀')).toBe('😀😀');
		expect(searchTerm('😀'.repeat(100))).toBe('😀'.repeat(100));
	});
});

describe('requestSlug', () => {
	const row = (over: Partial<WidgetRow>): WidgetRow => ({
		slug: 'request',
		kind: 'request',
		name: 'Request',
		monogram: 'U',
		icon: null,
		color: '#000',
		hostId: 'mini-pc',
		hasUrl: true,
		support: 'ready',
		state: 'ok',
		dataAt: null,
		failingSince: null,
		...over
	});

	it('trouve la connexion Request, seulement si elle est connectée', () => {
		expect(requestSlug([row({ slug: 'emby', kind: 'emby' }), row({ slug: 'demandes' })])).toBe(
			'demandes'
		);
		expect(requestSlug([row({ state: '' })])).toBeUndefined();
		expect(requestSlug([row({ state: 'unsupported' })])).toBeUndefined();
		// Le nom du service ne prouve rien : seul le connecteur compte.
		expect(requestSlug([row({ slug: 'request', kind: undefined })])).toBeUndefined();
	});
});

describe('affiches', () => {
	const pending: PendingRequest = {
		id: 1,
		mediaType: 'tv',
		tmdbId: 5,
		title: 'Une série',
		poster: 'a.jpg',
		requestedBy: 'Alice',
		is4k: true,
		seasons: [1, 2],
		createdAt: '2026-11-06T10:00:00Z'
	};

	it('habille une demande à approuver, l’image passant par Hub', () => {
		const poster = pendingPoster(pending, 'request');
		expect(poster.when).toBe('Série · S1, S2');
		expect(poster.detail).toBe('Alice · 4K');
		expect(poster.image).toBe('/api/v1/posters/request/a.jpg');
		expect(
			pendingPoster({ ...pending, mediaType: 'movie', seasons: [], year: 1999 }, 'x').when
		).toBe('Film · 1999');
	});

	it('habille un résultat de recherche, et ne propose de demander que l’inconnu', () => {
		const item: RequestSearchItem = {
			mediaType: 'movie',
			tmdbId: 1,
			title: 'Matrix',
			year: 1999,
			status: null
		};
		expect(searchPoster(item, 'request')).toMatchObject({
			title: 'Matrix',
			detail: '1999',
			when: 'Film',
			image: undefined
		});
		expect(isRequestable(item)).toBe(true);
		expect(isRequestable({ ...item, status: 'available' })).toBe(false);
	});
});

describe('sparkline', () => {
	it('ne trace rien sans mesure', () => {
		expect(sparkline([], 100, 20)).toEqual({ line: '', area: '', end: null });
	});

	it('monte avec la série et se ferme sur la base', () => {
		const shape = sparkline([0, 5, 10], 108, 28, 4);
		expect(shape.line).toBe('M4 24 L54 14 L104 4');
		expect(shape.area).toBe('M4 24 L54 14 L104 4 L104 24 L4 24 Z');
		expect(shape.end).toEqual({ x: 104, y: 4 });
	});

	it('reste à plat sur une série nulle, et trace un trait pour un seul point', () => {
		expect(sparkline([0, 0, 0], 108, 28, 4).line).toBe('M4 24 L54 24 L104 24');
		expect(sparkline([7], 108, 28, 4).line).toBe('M4 4 L104 4');
	});

	it('résume la courbe pour qui ne la voit pas', () => {
		const byMonth = [
			{ month: '2026-01', count: 2 },
			{ month: '2026-02', count: 9 }
		];
		expect(sparklineSummary(byMonth)).toBe(
			'11 demandes sur 2 mois, le plus actif : février 2026 avec 9.'
		);
		expect(sparklineSummary([])).toBe('Aucune demande sur la période.');
		expect(sparklineSummary([{ month: '2026-01', count: 0 }])).toBe('0 demande sur 1 mois.');
	});
});

describe('fourKShare', () => {
	it('arrondit la part de 4K, et se tait sans demande', () => {
		expect(fourKShare({ standard: 110, fourK: 10 })).toBe(8);
		expect(fourKShare({ standard: 0, fourK: 0 })).toBeNull();
	});
});
