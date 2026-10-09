import { parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const root = resolve(import.meta.dirname, "../../..");
const databases = [
  { name: "SQLite", driver: "sqlite", variable: "" },
  { name: "MySQL", driver: "mysql", variable: "FORMS_DB_MYSQL_DSN" },
  { name: "MariaDB", driver: "mysql", variable: "FORMS_DB_MARIADB_DSN" },
  { name: "PostgreSQL", driver: "postgres", variable: "FORMS_DB_POSTGRES_DSN" },
] as const;

describe("forms-db concurrent replica startup", () => {
  for (const database of databases) {
    const dsn = database.variable ? process.env[database.variable] : undefined;
    it.skipIf(Boolean(database.variable && !dsn))(`safely initializes shared storage when ${database.name} replicas start together`, () => {
      const output = execFileSync("go", ["run", "./tests/fixtures/sql/startup"], {
        cwd: root,
        encoding: "utf8",
        timeout: 180_000,
        env: {
          ...process.env,
          GOWORK: "off",
          GOTOOLCHAIN: "go1.26.0",
          FORMS_STARTUP_SQL_DRIVER: database.driver,
          FORMS_STARTUP_SQL_DSN: dsn ?? "",
        },
      });
      expect(parseReport(output)).toEqual({
        databaseDriver: database.driver,
        simultaneousInitializations: 12,
        allReplicasOpened: true,
        crossReplicaReadAfterWrite: true,
      });
    }, 180_000);
  }
});
