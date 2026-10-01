import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const repository = resolve(import.meta.dirname, '../..');

describe('forms-db v1 lifecycle ownership', () => {
  it('builds independently without the retired pluginprotocol lifecycle', () => {
    expect(() => execFileSync('go', ['build', './...'], {
      cwd: repository,
      env: { ...process.env, GOWORK: 'off' },
      stdio: 'pipe',
    })).not.toThrow();

    const module = readFileSync(resolve(repository, 'go.mod'), 'utf8');
    expect(module).toContain('github.com/Liapoldus/plugin-sdk');
  }, 120_000);
});
