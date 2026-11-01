import { getService } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ fetch, params }) => ({ service: getService(fetch, params.slug) });
