import { readContract, parseReport } from '../helpers/contracts.ts';
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');
const limits = readContract('runtime-limits.json');

describe('forms-db v1 resource limits', () => {
  it('publishes the approved settings, schema, submission, and database bounds', () => {
    expect(limits).toMatchObject({
      version: 1,
      settingsMaxBytes: 262_144,
      schemaMaxDepth: 32,
      fieldsPerFormMax: 128,
      submissionRequestMaxBytes: 1_048_576,
      databaseTimeoutMilliseconds: 5_000,
      databaseConcurrentOperationsMax: 64,
    });
  });

  it('keeps list page and cursor boundaries aligned with their owner contracts', () => {
    const listSchema = readContract('list-request.schema.json');
    const cursor = readContract('cursor.json');

    expect(listSchema.properties.limit.maximum).toBe(100);
    expect(cursor.maxPageSize).toBe(100);
    expect(listSchema.properties.cursor.maxLength).toBe(4096);
    expect(cursor.maxTokenBytes).toBe(4096);
    // Native tests/unit/storage_definitions_test.go verifies the 101-row
    // lookahead bound against memory and SQLite repositories.
  });

  it('enforces the byte, schema-depth, field-count and submission limits in runtime code', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/validation/limits'], {
      cwd: root,
      encoding: 'utf8',
      env: { ...process.env, GOWORK: 'off', GOTOOLCHAIN: 'go1.26.0' },
      timeout: 120_000,
    });

    expect(parseReport(output)).toEqual({
      settingsBytesAtLimitAccepted: true,
      settingsBytesOverLimitRejected: true,
      manifestSettingsAtLimitAccepted: true,
      manifestSettingsOverLimitRejected: true,
      schemaDepthAtLimitAccepted: true,
      schemaDepthOverLimitRejected: true,
      schemaFieldsAtLimitAccepted: true,
      schemaFieldsOverLimitRejected: true,
      submissionAtLimitAccepted: true,
      submissionOverLimitRejected: true,
    });
  });
});
