import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("../..", import.meta.url));

describe("forms-db-owned admin surface contract", () => {
  it("declares only product capabilities; common settings belong to Core API", async () => {
    const surfaceSource = await readFile(`${root}/contracts/v1/admin-surface.json`, "utf8");
    const surface = JSON.parse(surfaceSource);
    expect(surface.requiredCapabilities).toEqual(["forms.list", "forms.delete"]);
    expect(surface.pages.map((page: { id: string }) => page.id)).toEqual(["submissions"]);
    expect(surfaceSource).not.toContain("ConfigApply");
    expect(surfaceSource).not.toContain("ConfigSchema");
  });

  it("maps the submissions query and delete UI operations to plugin-owned actions", async () => {
    const [surfaceSource, listSchemaSource, deleteSchemaSource] = await Promise.all([
      readFile(`${root}/contracts/v1/admin-surface.json`, "utf8"),
      readFile(`${root}/contracts/v1/list-request.schema.json`, "utf8"),
      readFile(`${root}/contracts/v1/delete-request.schema.json`, "utf8"),
    ]);
    const surface = JSON.parse(surfaceSource);
    const listSchema = JSON.parse(listSchemaSource);
    const deleteSchema = JSON.parse(deleteSchemaSource);
    const submissions = surface.pages.find((page: { id: string }) => page.id === "submissions");

    expect(submissions.query).toEqual({
      id: "query",
      capability: "forms.list",
      inputSchema: {
        type: listSchema.type,
        properties: listSchema.properties,
        required: listSchema.required,
        additionalProperties: listSchema.additionalProperties,
      },
    });
    const records = submissions.sections.find((section: { id: string }) => section.id === "records");
    const action = records.actions.find((candidate: { id: string }) => candidate.id === "delete");
    expect(action.capability).toBe("forms.delete");
    expect(action.inputSchema).toEqual({
      type: deleteSchema.type,
      properties: deleteSchema.properties,
      required: deleteSchema.required,
      additionalProperties: deleteSchema.additionalProperties,
    });
    expect(surfaceSource).not.toContain("admin.surface.get");
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
