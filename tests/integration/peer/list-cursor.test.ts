import { readContract, parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../../..');
const requestJSONVectors = readFileSync(resolve(root, 'contracts/v1/request-json-vectors.json'), 'utf8');
const submitNegativeVectors = readFileSync(resolve(root, 'contracts/v1/submit-negative-vectors.json'), 'utf8');
const replicaVectors = readContract('cursor-replica-vectors.json');

describe('forms-db peer list cursor', () => {
  it('pages without offset and refuses a tampered cursor', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/peer/list'], {
      cwd: root,
      env: {
        ...process.env,
        GOWORK: 'off',
        FORMS_REQUEST_JSON_VECTORS: requestJSONVectors,
        FORMS_SUBMIT_NEGATIVE_VECTORS: submitNegativeVectors,
      },
      encoding: 'utf8',
    });
    const report = parseReport(output);
    expect(report).toMatchObject({
      pageOne: true,
      pageTwo: true,
      tamperRefused: true,
      hostileFilterRefused: true,
      cursorReplayAccepted: true,
      secretScoped: true,
      invalidRequestVectorsRefused: true,
      submitNegativeVectorsRefused: true,
      crossReplicaPage: true,
    });
    const rotatedKeyVector = replicaVectors.vectors.find((vector: { name: string }) => vector.name === 'old-cursor-after-key-rotation');
    if (!rotatedKeyVector) throw new Error("missing rotatedKeyVector contract entry");
    const unavailableGrantVector = replicaVectors.vectors.find((vector: { name: string }) => vector.name === 'cursor-grant-unavailable-on-one-replica');
    if (!unavailableGrantVector) throw new Error("missing unavailableGrantVector contract entry");
    const unsupportedVersionVector = replicaVectors.vectors.find((vector: { name: string }) => vector.name === 'unsupported-token-version');
    if (!unsupportedVersionVector) throw new Error("missing unsupportedVersionVector contract entry");
    expect(report.rotatedKeyStatus, rotatedKeyVector.name).toBe(rotatedKeyVector.expectedStatus);
    expect(report.unavailableGrantStatus, unavailableGrantVector.name).toBe(unavailableGrantVector.expectedStatus);
    expect(report.unsupportedVersionStatus, unsupportedVersionVector.name).toBe(unsupportedVersionVector.expectedStatus);
    expect(replicaVectors.semantics.replay).toContain('reusable read-only continuation token');
    expect(replicaVectors.vectors).toContainEqual({ name: 'shared-current-key-cross-replica', expectedStatus: 200 });
    expect(replicaVectors.vectors).toContainEqual({ name: 'valid-cursor-replay', expectedStatus: 200 });
  }, 30_000);
});
