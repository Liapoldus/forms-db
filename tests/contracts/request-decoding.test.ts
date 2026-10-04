import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const contract = JSON.parse(readFileSync(resolve(import.meta.dirname, '../../contracts/v1/request-json-vectors.json'), 'utf8'));

describe('forms-db request JSON contract', () => {
  it('defines duplicate-key and trailing-document failures as validation errors', () => {
    expect(contract.version).toBe(1);
    expect(contract.invalidRequests).toHaveLength(6);
    expect(new Set(contract.invalidRequests.map((testCase: { name: string }) => testCase.name)).size).toBe(6);
    for (const testCase of contract.invalidRequests) {
      expect(['forms.submit', 'forms.list', 'forms.delete']).toContain(testCase.capability);
      expect(testCase.expectedStatus).toBe(422);
      expect(testCase.expectedCode).toBe('validation_failed');
    }
  });
});
