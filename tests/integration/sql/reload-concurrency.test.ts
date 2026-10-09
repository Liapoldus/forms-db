import { parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');

describe('forms-db SQL repository replacement', () => {
  it('keeps in-flight SQL calls on the old generation and serves from the candidate after Reload', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/sql/reload'], {
      cwd: root,
      env: { ...process.env, GOWORK: 'off', GOTOOLCHAIN: 'go1.26.0' },
      encoding: 'utf8',
      timeout: 60_000,
    });

    expect(parseReport(output)).toEqual({
      inFlightCallUsedOldSQLRepository: true,
      reloadWaitedForInFlightCall: true,
      candidateGenerationServedAfterReload: true,
      previousSQLRepositoryClosedAfterSwap: true,
    });
  });
});
