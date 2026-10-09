import { parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const root = resolve(import.meta.dirname, "../../..");
const databases = [
  { name: "MySQL", driver: "mysql", variable: "FORMS_DB_MYSQL_DSN" },
  { name: "MariaDB", driver: "mysql", variable: "FORMS_DB_MARIADB_DSN" },
  { name: "PostgreSQL", driver: "postgres", variable: "FORMS_DB_POSTGRES_DSN" },
] as const;

describe("forms-db shared SQL replica compatibility", () => {
  for (const database of databases) {
    const dsn = process.env[database.variable];
    it.skipIf(!dsn)(`continues a cursor and fences generations across ${database.name} replicas`, () => {
      const output = execFileSync("go", ["run", "./tests/fixtures/cursor/replicas"], {
        cwd: root,
        encoding: "utf8",
        timeout: 180_000,
        env: {
          ...process.env,
          GOWORK: "off",
          GOTOOLCHAIN: "go1.26.0",
          FORMS_MULTI_REPLICA_SQL_DRIVER: database.driver,
          FORMS_MULTI_REPLICA_SQL_DSN: dsn,
        },
      });
      expect(parseReport(output)).toEqual({
        databaseDriver: database.driver,
        firstReplicaReady: true,
        secondReplicaReady: true,
        cursorContinuedAcrossProcesses: true,
        cursorContinuedAcrossMixedGenerations: true,
        firstReplicaAdvanced: true,
        secondReplicaRemainedOnPreviousGeneration: true,
        processGenerationFence: true,
      });
    }, 180_000);
  }
});
