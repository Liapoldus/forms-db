import { readContract } from '../helpers/contracts.ts';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const cursorSource = readFileSync('internal/infrastructure/security/cursor.go', 'utf8');
const cursorContract = readContract('cursor.json');
const peerSource = readFileSync('internal/presentation/peerplugin/list.go', 'utf8');
const restSource = readFileSync('internal/presentation/restplugin/adapter.go', 'utf8');
const runtimeSource = readFileSync('internal/presentation/restplugin/runtime.go', 'utf8');
const mainSource = readFileSync('cmd/forms-db/main.go', 'utf8');

await test('cursor signing key is obtained per call through Plugin SDK, never from plugin environment', () => {
  assert.equal('keyFileEnvironment' in cursorContract, false);
  assert.equal(cursorContract.grantCapability, 'forms.list');
  assert.equal(cursorContract.grantPurpose, 'cursor-signing');
  assert.equal(cursorSource.includes('os.Getenv('), false);
  assert.equal(mainSource.includes('os.Getenv('), false);
  assert.match(peerSource, /handler\.secrets\.SecretProvider\(/);
  assert.match(peerSource, /defer value\.Destroy\(\)/);
  assert.match(runtimeSource, /sdkapp\.NewSecretManager\(/);
  assert.match(mainSource, /sdkinfra\.NewCoreSecretBroker\(/);
  assert.equal(mainSource.includes('LoadCursorSigner'), false);
});

await test('application DSN stays an opaque reference through Reload and is redeemed for candidate only', () => {
  const settings = readContract('secrets.json');
  assert.equal(settings.dsnDelivery, 'plugin-sdk-generation-scoped-grant');
  assert.equal(settings.sdkGrantScope, 'candidate-generation');
  assert.equal(settings.rawSecretInReload, false);
  assert.match(restSource, /settings\.DSNReference/);
  assert.match(restSource, /secrets\.SecretProvider\(/);
  assert.match(restSource, /defer value\.Destroy\(\)/);
  assert.doesNotMatch(restSource, /ConfigApply|BootstrapRequest/);
});
