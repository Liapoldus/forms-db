import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("../..", import.meta.url));

describe("forms-db-owned admin surface contract", () => {
  it("declares plugin capabilities and renders settings through control RPCs", async () => {
    const surfaceSource = await readFile(`${root}/contracts/v1/admin-surface.json`, "utf8");
    const surface = JSON.parse(surfaceSource);
    const storage = surface.pages.find((page: { id: string }) => page.id === "storage");

    expect(surface.requiredCapabilities).toEqual(["admin.surface.get", "forms.list", "forms.delete"]);
    expect(storage.control).toEqual({ settingsSchemaRpc: "ConfigSchema", settingsApplyRpc: "ConfigApply" });
    expect(storage.sections[0].fieldsFromControlRpc).toBe("ConfigSchema");
    expect(storage).not.toHaveProperty("capability");
  });
});
