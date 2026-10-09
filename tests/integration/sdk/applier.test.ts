import { parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../../..');

describe('forms-db SDK configuration applier', () => {
  it('builds a secret-backed candidate and preserves active state on refusal', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/sdk/applier'], {
      cwd: root,
      env: { ...process.env, GOWORK: 'off' },
      encoding: 'utf8',
    });
    expect(parseReport(output)).toEqual({
      accepted: true,
      preserved: true,
      buildFailurePreserved: true,
      servesPriorState: true,
      secretWasScoped: true,
    });
  }, 30_000);
});
