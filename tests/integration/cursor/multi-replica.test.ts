import { parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../../..');

describe('forms-db independent process replicas', () => {
  it('continues a cursor across mTLS plugin processes and fences generations independently', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/cursor/replicas'], {
      cwd: root,
      encoding: 'utf8',
      timeout: 120_000,
      env: { ...process.env, GOWORK: 'off', GOTOOLCHAIN: 'go1.26.0' },
    });
    expect(parseReport(output)).toEqual({
      databaseDriver: 'sqlite',
      firstReplicaReady: true,
      secondReplicaReady: true,
      cursorContinuedAcrossProcesses: true,
      firstReplicaAdvanced: true,
      secondReplicaRemainedOnPreviousGeneration: true,
      processGenerationFence: true,
      cursorContinuedAcrossMixedGenerations: true,
    });
  }, 120_000);
});
