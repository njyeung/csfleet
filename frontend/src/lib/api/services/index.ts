// Barrel for the typed API services. Each is an Angular-service-style singleton
// grouping the endpoints for one resource. Import what you need:
//   import { servers, clusters } from '$lib/api/services';
export { servers } from './servers';
export { clusters } from './clusters';
export { plugins } from './plugins';
export { configs } from './configs';
export { env } from './env';
export { globals } from './globals';
export { auth } from './auth';
