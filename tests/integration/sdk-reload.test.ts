import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');

describe('forms-db SDK REST Reload', () => {
  it('pulls exact config, redeems candidate DSN and ACKs only applied generation', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/sdk-reload'], {
      cwd: root,
      env: { ...process.env, GOWORK: 'off' },
      encoding: 'utf8',
    });
    expect(JSON.parse(output)).toEqual({ applied: true, exactPull: true, candidateGrant: true, ready: true });
  }, 30_000);
});
