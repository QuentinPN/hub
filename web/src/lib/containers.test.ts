import { expect, it } from 'vitest';
import type { ServiceContainer, ServiceContainers } from './api/types';
import { labelsLabel, portLabel, splitImage, splitName, splittable } from './containers';

it('sépare l’image de son tag', () => {
	expect(splitImage('ghcr.io/immich-app/immich-server:v1.120.2')).toEqual({
		name: 'ghcr.io/immich-app/immich-server',
		tag: 'v1.120.2'
	});
	expect(splitImage('registry:5000/app')).toEqual({ name: 'registry:5000/app', tag: 'latest' });
	expect(splitImage('postgres')).toEqual({ name: 'postgres', tag: 'latest' });
	expect(splitImage('postgres@sha256:0123456789abcdef0123')).toEqual({
		name: 'postgres',
		tag: 'sha256:0123456789ab'
	});
});

it('dit les ports et les labels sans valeur', () => {
	expect(portLabel({ ip: '0.0.0.0', public: 8080, private: 80, type: 'tcp' })).toBe(
		'8080 → 80/tcp'
	);
	expect(portLabel({ ip: '127.0.0.1', public: 5432, private: 5432, type: 'tcp' })).toBe(
		'127.0.0.1:5432 → 5432/tcp'
	);
	expect(labelsLabel(18, 2)).toBe('18, dont 2 masqués');
	expect(labelsLabel(13, 1)).toBe('13, dont 1 masqué');
	expect(labelsLabel(9, 0)).toBe('9, aucun masqué');
	expect(labelsLabel(0, 0)).toBe('Aucun');
});

it('propose un nom au service mis à part', () => {
	expect(splitName('AnimeSchedule', 'anime-web')).toBe('AnimeSchedule Web');
	expect(splitName('Immich', 'machine_learning')).toBe('Immich Machine Learning');
	expect(splitName('Web', 'web')).toBe('Web Web');
});

it('ne propose la séparation que si le projet garde un service', () => {
	const c = (composeService: string, extra: Partial<ServiceContainer> = {}) =>
		({
			composeProject: 'anime',
			composeService,
			linked: false,
			role: 'support',
			...extra
		}) as ServiceContainer;
	const data = (containers: ServiceContainer[], split: ServiceContainers['split'] = null) =>
		({ containers, split }) as ServiceContainers;
	expect([...splittable(data([c('admin', { role: 'main' }), c('web')]))]).toEqual(['admin', 'web']);
	expect(splittable(data([c('admin'), c('admin')])).size).toBe(0);
	expect(splittable(data([c('admin'), c('migrate', { role: 'oneshot' })])).size).toBe(0);
	expect(splittable(data([c('admin'), c('db', { linked: true })])).size).toBe(0);
	const from = {
		project: 'anime',
		service: 'web',
		since: '',
		parentSlug: 'anime',
		parentName: 'A'
	};
	expect(splittable(data([c('admin'), c('web')], from)).size).toBe(0);
});
