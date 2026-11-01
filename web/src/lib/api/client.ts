import type { Discovery, Home, ServiceDetail, ServiceSummary, Settings } from './types';

export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string
	) {
		super(message);
	}
}

type Fetch = typeof fetch;

async function getJson<T>(fetchFn: Fetch, path: string): Promise<T> {
	const res = await fetchFn(path, { headers: { accept: 'application/json' } });
	if (!res.ok) {
		throw new ApiError(res.status, `GET ${path} → ${res.status}`);
	}
	return (await res.json()) as T;
}

async function putJson<T>(fetchFn: Fetch, path: string, body: unknown): Promise<T> {
	const res = await fetchFn(path, {
		method: 'PUT',
		headers: { accept: 'application/json', 'content-type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!res.ok) {
		// Le serveur renvoie {"error": "..."} ; un corps illisible garde le message générique.
		const detail = await res
			.json()
			.then((b: { error?: string }) => b.error)
			.catch(() => undefined);
		throw new ApiError(res.status, detail ?? `PUT ${path} → ${res.status}`);
	}
	return (await res.json()) as T;
}

export const getHome = (fetchFn: Fetch) => getJson<Home>(fetchFn, '/api/v1/home');

export const getServices = (fetchFn: Fetch) =>
	getJson<ServiceSummary[]>(fetchFn, '/api/v1/services');

export const getSettings = (fetchFn: Fetch) => getJson<Settings>(fetchFn, '/api/v1/settings');

export const getService = (fetchFn: Fetch, slug: string) =>
	getJson<ServiceDetail>(fetchFn, `/api/v1/services/${encodeURIComponent(slug)}`);

export const updateDiscovery = (fetchFn: Fetch, discovery: Discovery) =>
	putJson<Settings>(fetchFn, '/api/v1/settings/discovery', discovery);
