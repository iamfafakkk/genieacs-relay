import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
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
			// ws: true lets the /jobs/ws upgrade through to the backend too.
			proxy: {
				'^/(api|healthz?|readyz?|version|metrics|swagger)(/|$)': { target, ws: true }
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

				// The Go binary serves web/dist directly, so the build is a pure
				// client-side SPA: index.html is the fallback for every route.
				adapter: adapter({
					pages: 'dist',
					assets: 'dist',
					fallback: 'index.html'
				})
			})
		]
	};
});
