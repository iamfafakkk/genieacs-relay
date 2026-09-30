import { redirect } from '@sveltejs/kit';
import { KEY_COOKIE } from '$lib/api/client';
import type { LayoutServerLoad } from './$types';

// Route guard for the (admin) group. The API key is the only auth model, so
// "logged in" == key cookie present. The backend rejects a bad key with 401.
export const load: LayoutServerLoad = ({ cookies }) => {
	if (!cookies.get(KEY_COOKIE)) redirect(303, '/login');
};
