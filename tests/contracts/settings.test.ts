import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import Ajv2020 from 'ajv/dist/2020.js';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');
const schema = JSON.parse(readFileSync(resolve(root, 'contracts/v1/settings.schema.json'), 'utf8'));
const manifest = JSON.parse(readFileSync(resolve(root, 'contracts/v1/plugin.json'), 'utf8'));
const validate = new Ajv2020({ strict: true }).compile(schema);

describe('forms-db SDK settings contract', () => {
  it('publishes only plugin-owned capabilities and one settings schema', () => {
    expect(manifest.name).toBe('forms-db');
    expect(manifest.configuration.schema).toBe('contracts/v1/settings.schema.json');
    expect(manifest.capabilities.map((entry: { name: string }) => entry.name)).toEqual([
      'forms.submit', 'forms.list', 'forms.delete', 'admin.surface.get',
    ]);
  });

  it('accepts v1 persistent backends with opaque DSN references', () => {
    for (const driver of ['sqlite', 'mysql', 'postgres']) {
      expect(validate({ driver, dsn: 'secret:forms-storage', schemas: {} })).toBe(true);
    }
  });

  it('refuses unknown fields and missing persistent credentials', () => {
    expect(validate({ driver: 'postgres', schemas: {} })).toBe(false);
    expect(validate({ driver: 'postgres', dsn: 'secret:forms-storage', schemas: {}, endpoint: '127.0.0.1' })).toBe(false);
  });
});
