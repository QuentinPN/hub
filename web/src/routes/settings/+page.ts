import { getSettings } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ fetch }) => ({ settings: getSettings(fetch) });
