import { parseReport } from '../../helpers/contracts.ts';
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const root = resolve(import.meta.dirname, "../../..");

describe("forms-db mixed-generation SQL replicas", () => {
  it("continues a cursor across replicas after only one replica reloads", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/cursor/replicas"], {
      cwd: root,
      encoding: "utf8",
      timeout: 180_000,
      env: {
        ...process.env,
        GOWORK: "off",
        GOTOOLCHAIN: "go1.26.0",
        FORMS_MULTI_REPLICA_SQL_DRIVER: "sqlite",
      },
    });
    expect(parseReport(output)).toMatchObject({
      databaseDriver: "sqlite",
      firstReplicaAdvanced: true,
      secondReplicaRemainedOnPreviousGeneration: true,
      cursorContinuedAcrossMixedGenerations: true,
    });
  }, 180_000);
});
