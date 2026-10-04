import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');

describe('forms-db cursor runtime contract', () => {
	it('binds cursor to query scope, expiry, signing key and authenticated token bytes', () => {
		const output = execFileSync('go', ['run', './tests/fixtures/cursor-runtime'], {
			cwd: root,
			env: { ...process.env, GOWORK: 'off' },
			encoding: 'utf8',
		});
		expect(JSON.parse(output)).toEqual({
			valid: true,
			scopeBound: true,
			expires: true,
			rotationInvalidatesOldToken: true,
			versionTamperingRejected: true,
			ciphertextTamperingRejected: true,
		});
	}, 30_000);
});
