// Pure client-side SPA: no SSR, no prerender. The orchestrator is the only
// backend, reached over /api (proxied in dev — see vite.config.ts). Routes fall
// back to index.html via adapter-static.
export const ssr = false;
export const prerender = false;
