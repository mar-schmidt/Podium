// Shared TypeScript types mirroring the server data model. This file grows into
// the typed WS/REST contract in later phases; for now it covers the health
// endpoint the Phase 0 skeleton consumes.

export interface Health {
  status: string;
  version: string;
  commit: string;
  started: string;
  uptime_ms: number;
}
