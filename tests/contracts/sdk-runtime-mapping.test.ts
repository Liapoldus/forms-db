import { readFile } from 'node:fs/promises';
import { expect, it } from 'vitest';

const root = new URL('../../', import.meta.url);

it('passes the Plugin SDK artifact-stream deadline from its versioned HTTP contract', async () => {
  const runtime = await readFile(new URL('internal/presentation/restplugin/runtime.go', root), 'utf8');

  expect(runtime).toContain('Deadline: time.Duration(contract.Deadlines.ArtifactStreamSeconds) * time.Second');
});
