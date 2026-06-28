import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) => filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// SPA: every route falls back to the shell (see +layout.ts ssr=false). The
			// orchestrator serves the API; this app is pure client-side.
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	server: {
		// Forward /api (REST + the /api/events SSE stream) to the orchestrator so the
		// browser talks same-origin and we avoid CORS. Point this at wherever the Go
		// control plane listens (config.APIAddr, default :8080).
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true
			}
		}
	}
});
