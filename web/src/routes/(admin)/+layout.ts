import { redirect } from '@sveltejs/kit';
import { getApiKey } from '$lib/api/client';
import type { LayoutLoad } from './$types';

// Route guard for the (admin) group. The API key is the only auth model, so
// "logged in" == key cookie present. The backend rejects a bad key with 401.
export const load: LayoutLoad = () => {
	if (!getApiKey()) redirect(303, '/login');
};