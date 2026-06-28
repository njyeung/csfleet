import { request } from '../client';
import type {
  OrchestratorInfo
} from '../types';

// Orchestrator Info

export const orchestratorinfo = {
  get: () => request<OrchestratorInfo>('GET', '/api/orchestratorinfo')
};
