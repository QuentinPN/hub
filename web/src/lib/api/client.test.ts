import { describe, expect, it, vi } from 'vitest';
import { ApiError, getService, updateDiscovery } from './client';

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
