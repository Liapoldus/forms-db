import { readContract } from '../helpers/contracts.ts';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { describe, expect, it } from 'vitest';


const read = readContract;

function compile(schema: object) {
  const ajv = new Ajv2020({ allErrors: true, strict: false });
  addFormats(ajv);
  return ajv.compile(schema);
}

describe('forms.submit v1 contract', () => {
  it('validates the product-owned request and response shapes', () => {
    const request = compile(read('submit-request.schema.json'));
    const response = compile(read('submit-response.schema.json'));

    expect(request({ site: 'portal', schemaName: 'contact', data: { email: 'a@example.test' } })).toBe(true);
    expect(request({ site: 'portal', schemaName: 'contact', data: ['not', 'an', 'object'] })).toBe(false);
    expect(request({ site: 'portal', schemaName: 'contact', data: {}, unexpected: true })).toBe(false);
    expect(response({
      id: `frm_${'a'.repeat(24)}`,
      createdAt: '2026-10-02T12:00:00.123456789Z',
      data: { email: 'a@example.test' },
    })).toBe(true);
    expect(response({ id: 'bad', createdAt: 'not-a-time', data: {} })).toBe(false);
  });

  it('maps invalid submitted data and storage failures to stable safe errors', () => {
    expect(read('submit-errors.json')).toEqual({
      $id: 'https://github.com/Liapoldus/forms-db/contracts/v1/submit-errors.json',
      capability: 'forms.submit',
      version: 1,
      errors: {
        validation_failed: { http: 422, retryable: false },
        storage_unavailable: { http: 503, retryable: true },
      },
    });
  });

  it('keeps every negative request vector outside the published request schema', () => {
    const validate = compile(read('submit-request.schema.json'));
    const vectors = read('submit-negative-vectors.json') as {
      schema: string;
      cases: Array<{ name: string; payload: unknown; expectedStatus: number; expectedCode: string }>;
    };

    expect(vectors.schema).toBe('contracts/v1/submit-request.schema.json');
    expect(vectors.cases).toHaveLength(4);
    for (const vector of vectors.cases.filter((item) => item.name !== 'configured-schema-rejects-value')) {
      expect(validate(vector.payload), vector.name).toBe(false);
      expect(vector.expectedStatus).toBe(422);
      expect(vector.expectedCode).toBe('validation_failed');
    }
    const configuredSchemaCase = vectors.cases.find((item) => item.name === 'configured-schema-rejects-value');
    expect(configuredSchemaCase).toBeDefined();
    expect(validate(configuredSchemaCase?.payload)).toBe(true);
    expect(configuredSchemaCase?.expectedStatus).toBe(422);
    expect(configuredSchemaCase?.expectedCode).toBe('validation_failed');
  });
});
