import { parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../../..');
const deleteNegativeVectors = readFileSync(resolve(root, 'contracts/v1/delete-negative-vectors.json'), 'utf8');

describe('forms-db generic peer dispatch', () => {
  it('serves plugin-owned submit/delete methods without lifecycle RPCs', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/peer/dispatch'], {
      cwd: root,
      env: { ...process.env, GOWORK: 'off', FORMS_DELETE_NEGATIVE_VECTORS: deleteNegativeVectors },
      encoding: 'utf8',
    });
    expect(parseReport(output)).toEqual({
      submitted: true,
      deleted: true,
      absent: true,
      httpEnvelope: true,
      deleteNegativeVectorsRefused: true,
    });
  }, 30_000);
});
