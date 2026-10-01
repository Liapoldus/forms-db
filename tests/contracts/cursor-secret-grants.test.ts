import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const cursorSource = readFileSync('internal/infrastructure/security/cursor.go', 'utf8');
const cursorContract = JSON.parse(readFileSync('internal/infrastructure/security/contracts/cursor.json', 'utf8'));
const peerSource = readFileSync('internal/presentation/peerplugin/handler.go', 'utf8');
const restSource = readFileSync('internal/presentation/restplugin/adapter.go', 'utf8');
const runtimeSource = readFileSync('internal/presentation/restplugin/runtime.go', 'utf8');
const mainSource = readFileSync('cmd/forms-db/main.go', 'utf8');

test('cursor signing key is obtained per call through Plugin SDK, never from plugin environment', () => {
  assert.equal(cursorContract.keyFileEnvironment, undefined);
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

test('application DSN stays an opaque reference through Reload and is redeemed for candidate only', () => {
  const settings = JSON.parse(readFileSync('internal/infrastructure/config/contracts/secret-settings.json', 'utf8'));
  assert.equal(settings.dsnDelivery, 'plugin-sdk-generation-scoped-grant');
  assert.equal(settings.sdkGrantScope, 'candidate-generation');
  assert.equal(settings.rawSecretInReload, false);
  assert.match(restSource, /settings\.DSNReference/);
  assert.match(restSource, /secrets\.SecretProvider\(/);
  assert.match(restSource, /defer value\.Destroy\(\)/);
  assert.doesNotMatch(restSource, /ConfigApply|BootstrapRequest/);
});
