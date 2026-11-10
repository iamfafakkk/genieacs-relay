import { redirect } from '@sveltejs/kit';
import { getApiKey } from '$lib/api/client';
import type { PageLoad } from './$types';

// Already authenticated → skip the form.
export const load: PageLoad = () => {
	if (getApiKey()) redirect(303, '/');
};