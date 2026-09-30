import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
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

  it("requires read and write permissions for the page that exposes delete actions", async () => {
    const surfaceSource = await readFile(`${root}/contracts/v1/admin-surface.json`, "utf8");
    const surface = JSON.parse(surfaceSource);
    const submissions = surface.pages.find((page: { id: string }) => page.id === "submissions");

    expect(submissions.permissions).toEqual(["plugins.forms-db.read", "plugins.forms-db.write"]);
  });

  it("binds forms.delete inputs to declared columns in the selected submissions row", async () => {
    const [surfaceSource, requestSource, vectorsSource] = await Promise.all([
      readFile(`${root}/contracts/v1/admin-surface.json`, "utf8"),
      readFile(`${root}/contracts/v1/delete-request.schema.json`, "utf8"),
      readFile(`${root}/contracts/v1/admin-surface-vectors.json`, "utf8"),
    ]);
    const surface = JSON.parse(surfaceSource);
    const deleteRequest = JSON.parse(requestSource);
    const vectors = JSON.parse(vectorsSource) as Array<{
      name: string;
      selectedRow: Record<string, unknown>;
      expectedPayload: Record<string, unknown>;
    }>;
    const submissions = surface.pages.find((page: { id: string }) => page.id === "submissions");
    const records = submissions.sections.find((section: { id: string }) => section.id === "records");
    const action = records.actions.find((candidate: { id: string }) => candidate.id === "delete");
    const inputSchema = {
      type: deleteRequest.type,
      properties: deleteRequest.properties,
      required: deleteRequest.required,
      additionalProperties: deleteRequest.additionalProperties,
    };

    expect(records.columns).toEqual(["id", "site", "schemaName", "createdAt", "data"]);
    expect(action.inputSchema).toEqual(inputSchema);
    expect(action.rowInput).toEqual({ site: "site", schemaName: "schemaName", id: "id" });

    const columns = records.columns as string[];
    const rowInput = action.rowInput as Record<string, string>;
    const validateActionInput = new Ajv2020({ allErrors: true, strict: false }).compile(action.inputSchema);
    const validateDeleteRequest = new Ajv2020({ allErrors: true, strict: false }).compile(deleteRequest);

    expect(vectors.length).toBeGreaterThanOrEqual(1);
    for (const vector of vectors) {
      const payload = Object.fromEntries(
        Object.entries(rowInput).map(([input, column]) => [input, vector.selectedRow[column]]),
      );

      expect(Object.values(rowInput).every((column) => columns.includes(column)), vector.name).toBe(true);
      expect(payload, vector.name).toEqual(vector.expectedPayload);
      expect(validateActionInput(payload), vector.name).toBe(true);
      expect(validateDeleteRequest(payload), vector.name).toBe(true);
    }
  });
});
