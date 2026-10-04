import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');

describe('forms-db atomic repository replacement', () => {
  it('keeps the old repository alive until an in-flight call releases it', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/reload-concurrency'], {
      cwd: root,
      env: { ...process.env, GOWORK: 'off', GOTOOLCHAIN: 'go1.26.0' },
      encoding: 'utf8',
      timeout: 30_000,
    });

    expect(JSON.parse(output)).toEqual({
      oldRepositoryNotClosedWhileInFlight: true,
      applyWaitedForInvocation: true,
      oldRepositoryClosedAfterSwap: true,
      candidateServesAfterSwap: true,
    });
  });
});
