import { execFileSync } from 'node:child_process';
import { describe, expect, it } from 'vitest';

describe('forms-db SQL operation timeout', () => {
  it('cancels a SQL write blocked by a real SQLite lock at the configured timeout', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/sql-operation-timeout'], {
      cwd: process.cwd(),
      encoding: 'utf8',
      env: { ...process.env, GOWORK: 'off', GOTOOLCHAIN: 'go1.26.0' },
      timeout: 30_000,
    });

    expect(JSON.parse(output)).toMatchObject({ timedOut: true });
  }, 35_000);
});
