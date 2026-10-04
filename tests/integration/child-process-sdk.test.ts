import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("../..", import.meta.url));
const requestJSONVectors = readFileSync(`${root}/contracts/v1/request-json-vectors.json`, "utf8");
const submitNegativeVectors = readFileSync(`${root}/contracts/v1/submit-negative-vectors.json`, "utf8");
const deleteNegativeVectors = readFileSync(`${root}/contracts/v1/delete-negative-vectors.json`, "utf8");
const adminSurfaceVectors = readFileSync(`${root}/contracts/v1/admin-surface-vectors.json`, "utf8");

describe("forms-db child-process Plugin SDK lifecycle", () => {
  it("reloads exact settings over mTLS, redeems a scoped DSN and keeps SQLite data after restart", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/child-process"], {
      cwd: root,
      encoding: "utf8",
      timeout: 120_000,
      env: {
        ...process.env,
        GOWORK: "off",
        GOTOOLCHAIN: "go1.26.0",
        FORMS_REQUEST_JSON_VECTORS: requestJSONVectors,
        FORMS_SUBMIT_NEGATIVE_VECTORS: submitNegativeVectors,
        FORMS_DELETE_NEGATIVE_VECTORS: deleteNegativeVectors,
        FORMS_ADMIN_SURFACE_VECTORS: adminSurfaceVectors,
      },
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
      serverToForms: true,
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
