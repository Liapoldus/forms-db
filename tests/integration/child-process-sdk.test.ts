import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("../..", import.meta.url));

describe("forms-db child-process Plugin SDK lifecycle", () => {
  it("reloads exact settings over mTLS, redeems a scoped DSN and keeps SQLite data after restart", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/child-process"], {
      cwd: root,
      encoding: "utf8",
      timeout: 120_000,
      env: { ...process.env, GOWORK: "off", GOTOOLCHAIN: "go1.26.0" },
    });
    expect(JSON.parse(output)).toEqual({ reload: true, peerSubmit: true, sqliteRestart: true, serverToForms: true, unauthorizedCallerRejected: true });
  }, 120_000);
});
