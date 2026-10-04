import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const root = resolve(import.meta.dirname, "../..");
const requestJSONVectors = readFileSync(resolve(root, "contracts/v1/request-json-vectors.json"), "utf8");
const submitNegativeVectors = readFileSync(resolve(root, "contracts/v1/submit-negative-vectors.json"), "utf8");
const deleteNegativeVectors = readFileSync(resolve(root, "contracts/v1/delete-negative-vectors.json"), "utf8");
const adminSurfaceVectors = readFileSync(resolve(root, "contracts/v1/admin-surface-vectors.json"), "utf8");
const databaseCases = [
  { name: "MySQL", driver: "mysql", variable: "FORMS_DB_MYSQL_DSN", adminVariable: "FORMS_DB_MYSQL_ADMIN_DSN" },
  { name: "MariaDB", driver: "mysql", variable: "FORMS_DB_MARIADB_DSN", adminVariable: "FORMS_DB_MARIADB_ADMIN_DSN" },
  { name: "PostgreSQL", driver: "postgres", variable: "FORMS_DB_POSTGRES_DSN", adminVariable: "FORMS_DB_POSTGRES_ADMIN_DSN" },
] as const;

describe("forms-db real SQL backend child-process lifecycle", () => {
  for (const database of databaseCases) {
    const dsn = process.env[database.variable];
    const adminDSN = process.env[database.adminVariable];
    it.skipIf(!dsn)(`applies settings, performs CRUD and recovers records after restart on ${database.name}`, () => {
      const output = execFileSync("go", ["run", "./tests/fixtures/child-process"], {
        cwd: root,
        encoding: "utf8",
        timeout: 120_000,
        env: {
          ...process.env,
          GOWORK: "off",
          GOTOOLCHAIN: "go1.26.0",
          FORMS_CHILD_SQL_DRIVER: database.driver,
          FORMS_CHILD_SQL_DSN: dsn,
          FORMS_CHILD_SQL_ADMIN_DSN: adminDSN ?? "",
          FORMS_REQUEST_JSON_VECTORS: requestJSONVectors,
          FORMS_SUBMIT_NEGATIVE_VECTORS: submitNegativeVectors,
          FORMS_DELETE_NEGATIVE_VECTORS: deleteNegativeVectors,
          FORMS_ADMIN_SURFACE_VECTORS: adminSurfaceVectors,
        },
      });
      const result = JSON.parse(output);
      expect(result.databaseDriver).toBe(database.driver);
      expect(result.databaseRestartPersistence).toBe(true);
      expect(result.peerSubmit).toBe(true);
      expect(result.productRequestVectorsRefused).toBe(true);
      expect(result.candidateBuildFailurePreserved).toBe(true);
      expect(result.candidateSchemaFailurePreserved).toBe(true);
      expect(result.candidateSecretGrantFailurePreserved).toBe(true);
      expect(result.candidateMigrationFailurePreserved).toBe(Boolean(adminDSN));
      expect(result.candidateConnectionFailurePreserved).toBe(true);
    }, 120_000);
  }
});
