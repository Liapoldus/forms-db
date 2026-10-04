import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');
const adminSurfaceVectors = readFileSync(resolve(root, 'contracts/v1/admin-surface-vectors.json'), 'utf8');
const requestJSONVectors = readFileSync(resolve(root, 'contracts/v1/request-json-vectors.json'), 'utf8');
const submitNegativeVectors = readFileSync(resolve(root, 'contracts/v1/submit-negative-vectors.json'), 'utf8');
const deleteNegativeVectors = readFileSync(resolve(root, 'contracts/v1/delete-negative-vectors.json'), 'utf8');

describe('forms-db Plugin SDK Admin Surface over child-process mTLS', () => {
  it('discovers the product descriptor and serves its query/delete actions', () => {
    const output = execFileSync('go', ['run', './tests/fixtures/child-process'], {
      cwd: root,
      env: {
        ...process.env,
        GOWORK: 'off',
        FORMS_CHILD_SKIP_SERVER: '1',
        FORMS_ADMIN_SURFACE_VECTORS: adminSurfaceVectors,
        FORMS_REQUEST_JSON_VECTORS: requestJSONVectors,
        FORMS_SUBMIT_NEGATIVE_VECTORS: submitNegativeVectors,
        FORMS_DELETE_NEGATIVE_VECTORS: deleteNegativeVectors,
      },
      encoding: 'utf8',
      timeout: 120_000,
    });

    expect(JSON.parse(output)).toEqual({
      reload: true,
      databaseDriver: "sqlite",
      databaseRestartPersistence: true,
      candidateBuildFailurePreserved: true,
      candidateSchemaFailurePreserved: true,
      candidateSecretGrantFailurePreserved: true,
      candidateMigrationFailurePreserved: true,
      candidateConnectionFailurePreserved: false,
      cursorGrantsPerCall: true,
      spentGrantReplayRejected: true,
      previousGenerationStillServes: true,
      peerSubmit: true,
      sqliteRestart: true,
      serverToForms: false,
      wrongCapabilityRejected: true,
      unauthorizedCallerRejected: true,
      unauthorizedReadDeleteRejected: true,
      productRequestVectorsRefused: true,
      untrustedPeerCARejected: true,
      unauthorizedPayloadNotPersisted: true,
      adminSurface: true,
      adminSurfaceVectorRuntimeAccepted: true,
      adminList: true,
      adminDelete: true,
      adminDeleteMissing: true,
      noSensitiveOutput: true,
    });
  }, 120_000);
});
