import { getServices } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	await parent();
	return { services: getServices(fetch) };
};
