import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

type Contracts = {
 'admin-surface-request.schema.json': typeof import('../../contracts/v1/admin-surface-request.schema.json');
 'admin-surface-response.schema.json': typeof import('../../contracts/v1/admin-surface-response.schema.json');
 'admin-surface-vectors.json': typeof import('../../contracts/v1/admin-surface-vectors.json');
 'admin-surface.json': typeof import('../../contracts/v1/admin-surface.json');
 'cursor-replica-vectors.json': typeof import('../../contracts/v1/cursor-replica-vectors.json');
 'cursor.json': typeof import('../../contracts/v1/cursor.json');
 'delete-errors.json': typeof import('../../contracts/v1/delete-errors.json');
 'delete-negative-vectors.json': typeof import('../../contracts/v1/delete-negative-vectors.json');
 'delete-request.schema.json': typeof import('../../contracts/v1/delete-request.schema.json');
 'delete-response.schema.json': typeof import('../../contracts/v1/delete-response.schema.json');
 'list-errors.json': typeof import('../../contracts/v1/list-errors.json');
 'list-request.schema.json': typeof import('../../contracts/v1/list-request.schema.json');
 'list-response.schema.json': typeof import('../../contracts/v1/list-response.schema.json');
 'plugin.json': typeof import('../../contracts/v1/plugin.json');
 'request-json-vectors.json': typeof import('../../contracts/v1/request-json-vectors.json');
 'runtime-limits.json': typeof import('../../contracts/v1/runtime-limits.json');
 'secrets.json': typeof import('../../contracts/v1/secrets.json');
 'settings.schema.json': typeof import('../../contracts/v1/settings.schema.json');
 'submit-errors.json': typeof import('../../contracts/v1/submit-errors.json');
 'submit-negative-vectors.json': typeof import('../../contracts/v1/submit-negative-vectors.json');
 'submit-request.schema.json': typeof import('../../contracts/v1/submit-request.schema.json');
 'submit-response.schema.json': typeof import('../../contracts/v1/submit-response.schema.json');
 'validation.json': typeof import('../../contracts/v1/validation.json');
};

export function parseContract<K extends keyof Contracts>(name: K, text: string): Contracts[K] {
 // These checked-in artifacts are verified against their Go owners by check-generated.
 const value: unknown = JSON.parse(text);
 void name;
 return value as Contracts[K];
}

export function readContract<K extends keyof Contracts>(name: K): Contracts[K] {
 return parseContract(name, readFileSync(resolve(import.meta.dirname, '../../contracts/v1', name), 'utf8'));
}

export function parseReport(text: string): Record<string, unknown> {
 const value: unknown = JSON.parse(text);
 if (value === null || typeof value !== 'object' || Array.isArray(value)) {
   throw new Error('fixture report must be an object');
 }
 return value as Record<string, unknown>;
}
