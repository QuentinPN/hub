import { describe, expect, it, vi } from 'vitest';
import { ApiError, getService } from './client';

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
