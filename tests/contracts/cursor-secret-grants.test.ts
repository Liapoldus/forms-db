import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const cursorSource = readFileSync('internal/infrastructure/security/cursor.go', 'utf8');
const cursorContract = JSON.parse(readFileSync('internal/infrastructure/security/contracts/cursor.json', 'utf8'));
const pluginSource = readFileSync('internal/presentation/plugin/server.go', 'utf8');
const mainSource = readFileSync('cmd/forms-db/main.go', 'utf8');

test('cursor signing key is obtained only through a request-scoped protocol grant', () => {
  assert.equal(cursorContract.keyFileEnvironment, undefined);
  assert.equal(cursorContract.grantCapability, 'forms.list');
  assert.equal(cursorContract.grantPurpose, 'cursor-signing');
  assert.equal(typeof cursorContract.grantDomain, 'string');
  assert.notEqual(cursorContract.grantDomain, '');
  assert.equal(cursorSource.includes('os.Getenv('), false);
  assert.equal(cursorSource.includes('os.ReadFile('), false);
  assert.equal(mainSource.includes('os.Getenv('), false);
  assert.match(mainSource, /transport\.ListenInherited\(\)/);
  assert.match(pluginSource, /func \(s \*Server\) Bootstrap\(/);
  assert.match(pluginSource, /DialGrantBrokerFromBootstrapContext/);
  assert.match(pluginSource, /request\.GetGrants\(\)/);
  assert.match(pluginSource, /\.Redeem\(/);
  assert.equal(mainSource.includes('LoadCursorSigner'), false);
});

test('application secret settings are redeemed during ConfigApply, not sent as bytes', () => {
  const settingsContract = JSON.parse(readFileSync('internal/infrastructure/config/contracts/secret-settings.json', 'utf8'));
  assert.equal(settingsContract.dsnDelivery, 'opaque-reference-matched-to-config-grant');
  assert.match(pluginSource, /request\.GetSettingsRevision\(\)/);
  assert.match(pluginSource, /RedeemConfig\(/);
  assert.match(pluginSource, /request\.GetGrants\(\)/);
  assert.match(pluginSource, /secret_reference|GetSecretReference/);
  assert.match(pluginSource, /settings\.DSNReference/);
});
