import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-auto';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', '');
	// Dev proxy target - keep in sync with SERVER_ADDR in the repo's .env.
	const target = env.VITE_API_PROXY_TARGET || 'http://localhost:8080';

	return {
		server: {
			// Dev-only: same-origin calls to backend paths reach the Go server.
			// Key starting with ^ is a RegExp; SvelteKit routes (/, /login) stay local.
			proxy: {
				'^/(api|healthz?|readyz?|version|metrics|swagger)(/|$)': target
			}
		},
		plugins: [
			tailwindcss(),
			sveltekit({
				compilerOptions: {
					// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
					runes: ({ filename }) =>
						filename.split(/[/\\]/).includes('node_modules') ? undefined : true
				},

				// adapter-auto only supports some environments, see https://svelte.dev/docs/kit/adapter-auto for a list.
				// If your environment is not supported, or you settled on a specific environment, switch out the adapter.
				// See https://svelte.dev/docs/kit/adapters for more information about adapters.
				adapter: adapter()
			})
		]
	};
});
