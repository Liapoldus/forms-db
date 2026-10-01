import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');

describe('forms-db peer list cursor', () => {
  it('pages without offset and refuses a tampered cursor', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/peer-list-cursor'], {
      cwd: root,
      env: { ...process.env, GOWORK: 'off' },
      encoding: 'utf8',
    });
    expect(JSON.parse(output)).toEqual({ pageOne: true, pageTwo: true, tamperRefused: true, secretScoped: true });
  }, 30_000);
});
