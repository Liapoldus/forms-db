import { readdir, readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("..", import.meta.url));

async function goFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(entries.map(async (entry) => {
    if (entry.name === ".git" || entry.name === "node_modules") return [];
    const path = `${directory}/${entry.name}`;
    if (entry.isDirectory()) return goFiles(path);
    return entry.name.endsWith(".go") ? [path] : [];
  }));
  return nested.flat();
}

describe("pluginprotocol v2 Go module dependency", () => {
  it("uses the breaking module major without changing peer wire v1", async () => {
    const goMod = await readFile(`${root}/go.mod`, "utf8");
    expect(goMod).toMatch(/github\.com\/Liapoldus\/pluginprotocol\/v2 v2\.0\.0/);
    for (const file of await goFiles(root)) {
      const source = await readFile(file, "utf8");
      expect(source, file).not.toMatch(/github\.com\/Liapoldus\/pluginprotocol\/(?!v2\/)/);
    }
  });
});
