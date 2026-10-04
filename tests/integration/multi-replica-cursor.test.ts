import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');

describe('forms-db independent process replicas', () => {
  it('continues a cursor across mTLS plugin processes and fences generations independently', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/multi-replica-cursor'], {
      cwd: root,
      encoding: 'utf8',
      timeout: 120_000,
      env: { ...process.env, GOWORK: 'off', GOTOOLCHAIN: 'go1.26.0' },
    });
    expect(JSON.parse(output)).toEqual({
      firstReplicaReady: true,
      secondReplicaReady: true,
      cursorContinuedAcrossProcesses: true,
      firstReplicaAdvanced: true,
      secondReplicaRemainedOnPreviousGeneration: true,
      processGenerationFence: true,
    });
  }, 120_000);
});
