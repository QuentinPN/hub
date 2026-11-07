import type { ContainerRole, PublishedPort, ServiceContainer } from './api/types';

/** Sépare l'image de son tag : `ghcr.io:443/org/app:v1` → `ghcr.io:443/org/app` et `v1`. */
export function splitImage(image: string): { name: string; tag: string } {
	const at = image.indexOf('@');
	if (at >= 0) return { name: image.slice(0, at), tag: image.slice(at + 1, at + 20) };
	const colon = image.lastIndexOf(':');
	if (colon > image.lastIndexOf('/'))
		return { name: image.slice(0, colon), tag: image.slice(colon + 1) };
	return { name: image, tag: 'latest' };
}

export const roleLabels: Record<ContainerRole, string> = {
	main: 'Principal',
	support: 'Appoint',
	oneshot: 'Tâche unique',
	'': ''
};

interface HealthView {
	label: string;
	tone: 'ok' | 'warn' | 'danger' | 'none';
}

const health: Record<ServiceContainer['health'], HealthView> = {
	healthy: { label: 'Sain', tone: 'ok' },
	starting: { label: 'Démarrage', tone: 'warn' },
	unhealthy: { label: 'En échec', tone: 'danger' },
	'': { label: 'Pas de healthcheck', tone: 'none' }
};

export const healthView = (h: ServiceContainer['health']) => health[h];

/** Couleur de la pastille ; grise quand l'hôte ne répond plus, l'état n'étant plus sûr. */
export function stateColor(c: ServiceContainer, hostKnown: boolean): string {
	if (!hostKnown) return 'var(--status-unknown)';
	if (c.state === 'running')
		return c.health === 'unhealthy' ? 'var(--status-down)' : 'var(--status-online)';
	if (c.state === 'paused' || c.state === 'restarting') return 'var(--status-degraded)';
	return 'var(--status-unknown)';
}

/** « 2283 → 2283/tcp », l'adresse en tête si le port n'écoute pas partout. */
export function portLabel(p: PublishedPort): string {
	const ip = p.ip && p.ip !== '0.0.0.0' && p.ip !== '::' ? `${p.ip}:` : '';
	return `${ip}${p.public} → ${p.private}/${p.type}`;
}

export function labelsLabel(labels: number, masked: number): string {
	if (labels === 0) return 'Aucun';
	return `${labels}, ${masked === 0 ? 'aucun masqué' : masked === 1 ? 'dont 1 masqué' : `dont ${masked} masqués`}`;
}

const date = new Intl.DateTimeFormat('fr-FR', { day: 'numeric', month: 'short', year: 'numeric' });
export const formatDate = (iso: string) => date.format(new Date(iso));

export interface ProjectGroup {
	key: string;
	linked: boolean;
	containers: ServiceContainer[];
}

/** Regroupe par projet, dans l'ordre du serveur : le projet du service d'abord. */
export function byProject(list: ServiceContainer[]): ProjectGroup[] {
	const groups: ProjectGroup[] = [];
	for (const c of list) {
		let g = groups.find((x) => x.key === c.project);
		if (!g) groups.push((g = { key: c.project, linked: c.linked, containers: [] }));
		g.containers.push(c);
	}
	return groups;
}
