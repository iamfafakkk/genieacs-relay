import { redirect } from '@sveltejs/kit';
import { KEY_COOKIE } from '$lib/api/client';
import type { PageServerLoad } from './$types';

// Already authenticated → skip the form.
export const load: PageServerLoad = ({ cookies }) => {
	if (cookies.get(KEY_COOKIE)) redirect(303, '/');
};
