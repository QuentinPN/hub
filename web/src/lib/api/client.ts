import type {
	AuthState,
	Discovery,
	Home,
	PasskeyInfo,
	ServiceDetail,
	ServiceSummary,
	SessionInfo,
	Settings
} from './types';
import { createPasskey, getPasskey, type PasskeyOptions } from './webauthn';

export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string,
		/** Délai en secondes avant de réessayer (en-tête Retry-After d'un 429). */
		readonly retryAfter?: number
	) {
		super(message);
	}
}

type Fetch = typeof fetch;

// Routes publiques : leur 401 (mauvais mot de passe…) est géré par le formulaire qui les appelle.
const PUBLIC_AUTH = new Set([
	'/api/v1/auth/state',
	'/api/v1/auth/setup',
	'/api/v1/auth/login',
	'/api/v1/auth/passkey/login/begin',
	'/api/v1/auth/passkey/login/finish'
]);

// Appelé quand une route protégée répond 401 (session expirée ou révoquée). Le layout le
// branche sur la page de connexion ; le client lui-même ne connaît pas la navigation.
let onUnauthorized: (() => void) | null = null;

export function setUnauthorizedHandler(handler: (() => void) | null) {
	onUnauthorized = handler;
}

async function failure(res: Response, method: string, path: string): Promise<ApiError> {
	if (res.status === 401 && !PUBLIC_AUTH.has(path)) onUnauthorized?.();
	// Le serveur renvoie {"error": "..."} ; un corps illisible garde le message générique.
	const detail = await res
		.json()
		.then((b: { error?: string }) => b.error)
		.catch(() => undefined);
	const retry = Number(res.headers.get('retry-after'));
	return new ApiError(
		res.status,
		detail ?? `${method} ${path} → ${res.status}`,
		Number.isFinite(retry) && retry > 0 ? retry : undefined
	);
}

async function getJson<T>(fetchFn: Fetch, path: string): Promise<T> {
	const res = await fetchFn(path, { headers: { accept: 'application/json' } });
	if (!res.ok) throw await failure(res, 'GET', path);
	return (await res.json()) as T;
}

async function send(
	fetchFn: Fetch,
	method: 'PUT' | 'POST' | 'DELETE',
	path: string,
	body?: unknown
): Promise<Response> {
	const res = await fetchFn(path, {
		method,
		headers:
			body === undefined
				? { accept: 'application/json' }
				: { accept: 'application/json', 'content-type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body)
	});
	if (!res.ok) throw await failure(res, method, path);
	return res;
}

async function putJson<T>(fetchFn: Fetch, path: string, body: unknown): Promise<T> {
	return (await (await send(fetchFn, 'PUT', path, body)).json()) as T;
}

async function postJson<T>(fetchFn: Fetch, path: string, body?: unknown): Promise<T> {
	return (await (await send(fetchFn, 'POST', path, body)).json()) as T;
}

/** Requête dont la réponse attendue est un 204 sans corps. */
async function sendNoContent(
	fetchFn: Fetch,
	method: 'PUT' | 'POST' | 'DELETE',
	path: string,
	body?: unknown
): Promise<void> {
	await send(fetchFn, method, path, body);
}

export const getHome = (fetchFn: Fetch) => getJson<Home>(fetchFn, '/api/v1/home');

export const getServices = (fetchFn: Fetch) =>
	getJson<ServiceSummary[]>(fetchFn, '/api/v1/services');

export const getSettings = (fetchFn: Fetch) => getJson<Settings>(fetchFn, '/api/v1/settings');

export const getService = (fetchFn: Fetch, slug: string) =>
	getJson<ServiceDetail>(fetchFn, `/api/v1/services/${encodeURIComponent(slug)}`);

export const updateDiscovery = (fetchFn: Fetch, discovery: Discovery) =>
	putJson<Settings>(fetchFn, '/api/v1/settings/discovery', discovery);

export const getAuthState = (fetchFn: Fetch) => getJson<AuthState>(fetchFn, '/api/v1/auth/state');

export const setup = (fetchFn: Fetch, code: string, username: string, password: string) =>
	sendNoContent(fetchFn, 'POST', '/api/v1/auth/setup', { code, username, password });

export const login = (fetchFn: Fetch, username: string, password: string) =>
	sendNoContent(fetchFn, 'POST', '/api/v1/auth/login', { username, password });

export const logout = (fetchFn: Fetch) => sendNoContent(fetchFn, 'POST', '/api/v1/auth/logout');

export const getSessions = (fetchFn: Fetch) =>
	getJson<SessionInfo[]>(fetchFn, '/api/v1/auth/sessions');

export const revokeSession = (fetchFn: Fetch, id: number) =>
	sendNoContent(fetchFn, 'DELETE', `/api/v1/auth/sessions/${id}`);

/** « others » garde cette session ; « all » la ferme aussi (déconnexion partout). */
export async function revokeSessions(fetchFn: Fetch, scope: 'others' | 'all'): Promise<number> {
	const res = await send(fetchFn, 'DELETE', `/api/v1/auth/sessions?scope=${scope}`);
	return ((await res.json()) as { ended: number }).ended;
}

/** Le serveur ferme toutes les autres sessions ; celle-ci reste ouverte. */
export const changePassword = (fetchFn: Fetch, current: string, next: string) =>
	sendNoContent(fetchFn, 'PUT', '/api/v1/auth/password', { current, new: next });

type Options = PasskeyOptions<Record<string, unknown>>;

/** Connexion par passkey : le navigateur propose les passkeys connues pour ce site. */
export async function loginWithPasskey(fetchFn: Fetch): Promise<void> {
	const options = await postJson<Options>(fetchFn, '/api/v1/auth/passkey/login/begin');
	const credential = await getPasskey(options);
	await sendNoContent(fetchFn, 'POST', '/api/v1/auth/passkey/login/finish', credential);
}

export const getPasskeys = (fetchFn: Fetch) =>
	getJson<PasskeyInfo[]>(fetchFn, '/api/v1/auth/passkeys');

/** Ajoute une passkey au compte. Le mot de passe est redemandé (403 s'il est faux). */
export async function addPasskey(
	fetchFn: Fetch,
	name: string,
	password: string
): Promise<PasskeyInfo> {
	const options = await postJson<Options>(fetchFn, '/api/v1/auth/passkeys/begin', { password });
	const credential = await createPasskey(options);
	return postJson<PasskeyInfo>(fetchFn, '/api/v1/auth/passkeys/finish', { name, credential });
}

export const deletePasskey = (fetchFn: Fetch, id: number) =>
	sendNoContent(fetchFn, 'DELETE', `/api/v1/auth/passkeys/${id}`);
