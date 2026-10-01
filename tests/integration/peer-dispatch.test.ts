import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');

describe('forms-db generic peer dispatch', () => {
  it('serves plugin-owned submit/delete methods without lifecycle RPCs', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/peer-dispatch'], {
      cwd: root,
      env: { ...process.env, GOWORK: 'off' },
      encoding: 'utf8',
    });
    expect(JSON.parse(output)).toEqual({ submitted: true, deleted: true, absent: true, httpEnvelope: true });
  }, 30_000);
});
