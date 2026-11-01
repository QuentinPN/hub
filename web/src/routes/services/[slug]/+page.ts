import { getService } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params, parent }) => {
	await parent();
	return { service: getService(fetch, params.slug) };
};
