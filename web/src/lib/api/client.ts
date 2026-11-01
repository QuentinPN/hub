import type { Home, ServiceDetail } from './types';

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

export const getHome = (fetchFn: Fetch) => getJson<Home>(fetchFn, '/api/v1/home');

export const getService = (fetchFn: Fetch, slug: string) =>
	getJson<ServiceDetail>(fetchFn, `/api/v1/services/${encodeURIComponent(slug)}`);
