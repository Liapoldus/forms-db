import { parseReport } from '../helpers/contracts.ts';
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');

describe('forms-db runtime settings decoder', () => {
	it('accepts one valid settings object and rejects ambiguous JSON documents', () => {
		const output = execFileSync('go', ['run', './tests/fixtures/validation/settings'], {
			cwd: root,
			env: { ...process.env, GOWORK: 'off' },
			encoding: 'utf8',
		});
		expect(parseReport(output)).toEqual({
			valid: true,
			duplicateTopLevel: false,
			duplicateNested: false,
			trailingDocument: false,
			unknownTopLevel: false,
			invalidUTF8: false,
		});
	}, 30_000);
});
