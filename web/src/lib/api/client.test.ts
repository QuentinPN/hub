import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	ApiError,
	getAuthState,
	getHome,
	getService,
	getSessions,
	login,
	revokeSession,
	setUnauthorizedHandler,
	updateDiscovery
} from './client';

describe('getService', () => {
	it('encode le slug et renvoie le JSON', async () => {
		const fetchFn = vi.fn(async () => new Response(JSON.stringify({ slug: 'a b' })));
		const svc = await getService(fetchFn as unknown as typeof fetch, 'a b');
		expect(fetchFn).toHaveBeenCalledWith('/api/v1/services/a%20b', expect.anything());
		expect(svc.slug).toBe('a b');
	});

	it('lève une ApiError avec le code HTTP', async () => {
		const fetchFn = vi.fn(async () => new Response('', { status: 404 }));
		await expect(getService(fetchFn as unknown as typeof fetch, 'x')).rejects.toBeInstanceOf(
			ApiError
		);
	});
});

describe('updateDiscovery', () => {
	const discovery = {
		followEvents: true,
		rescanMinutes: 15,
		onRemoval: 'absent_only',
		archiveAfterDays: 7,
		confirmBelowPercent: 80,
		inferUrlFromPorts: false
	} as const;

	it('envoie du JSON en PUT', async () => {
		const fetchFn = vi.fn(async () => new Response(JSON.stringify({ editable: true })));
		await updateDiscovery(fetchFn as unknown as typeof fetch, discovery);
		expect(fetchFn).toHaveBeenCalledWith(
			'/api/v1/settings/discovery',
			expect.objectContaining({ method: 'PUT', body: JSON.stringify(discovery) })
		);
	});

	it("remonte le message d'erreur du serveur", async () => {
		const fetchFn = vi.fn(
			async () => new Response(JSON.stringify({ error: 'out of range' }), { status: 400 })
		);
		await expect(updateDiscovery(fetchFn as unknown as typeof fetch, discovery)).rejects.toThrow(
			'out of range'
		);
	});
});

describe('auth', () => {
	const asFetch = (fn: unknown) => fn as typeof fetch;

	it('login envoie les identifiants en POST et accepte un 204', async () => {
		const fetchFn = vi.fn(async () => new Response(null, { status: 204 }));
		await login(asFetch(fetchFn), 'admin', 'secret');
		expect(fetchFn).toHaveBeenCalledWith(
			'/api/v1/auth/login',
			expect.objectContaining({
				method: 'POST',
				body: JSON.stringify({ username: 'admin', password: 'secret' })
			})
		);
	});

	it('expose Retry-After sur un 429', async () => {
		const fetchFn = vi.fn(
			async () =>
				new Response(JSON.stringify({ error: 'too many attempts' }), {
					status: 429,
					headers: { 'retry-after': '120' }
				})
		);
		const err = await login(asFetch(fetchFn), 'a', 'b').catch((e: unknown) => e);
		expect(err).toBeInstanceOf(ApiError);
		expect((err as ApiError).status).toBe(429);
		expect((err as ApiError).retryAfter).toBe(120);
	});

	it('revokeSession envoie un DELETE sans corps', async () => {
		const fetchFn = vi.fn(async () => new Response(null, { status: 204 }));
		await revokeSession(asFetch(fetchFn), 42);
		expect(fetchFn).toHaveBeenCalledWith(
			'/api/v1/auth/sessions/42',
			expect.objectContaining({ method: 'DELETE', body: undefined })
		);
	});

	describe('handler 401', () => {
		const onUnauthorized = vi.fn();
		const unauthorized = vi.fn(async () => new Response('', { status: 401 }));
		beforeEach(() => {
			onUnauthorized.mockClear();
			setUnauthorizedHandler(onUnauthorized);
		});
		afterEach(() => setUnauthorizedHandler(null));

		it('se déclenche sur une route protégée', async () => {
			await expect(getHome(asFetch(unauthorized))).rejects.toBeInstanceOf(ApiError);
			await expect(getSessions(asFetch(unauthorized))).rejects.toBeInstanceOf(ApiError);
			expect(onUnauthorized).toHaveBeenCalledTimes(2);
		});

		it('ne se déclenche pas sur un échec de connexion', async () => {
			await expect(login(asFetch(unauthorized), 'a', 'b')).rejects.toBeInstanceOf(ApiError);
			await expect(getAuthState(asFetch(unauthorized))).rejects.toBeInstanceOf(ApiError);
			expect(onUnauthorized).not.toHaveBeenCalled();
		});
	});
});
