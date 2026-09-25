# forms-db

Реализация плагина форм Liapoldus. Процесс объявляет capabilities
`forms.submit`, `forms.list`, `forms.delete` и `admin.surface.get`. `ConfigApply`
может выбрать постоянное хранилище SQLite; проверка отправок по переданным при
конфигурации JSON Schema Draft 2020-12 реализована. Схемы пока не сохраняются в
БД и должны приходить при каждом применении конфигурации. Equality-фильтры по
разрешённым верхнеуровневым полям активной schema и HMAC-protected cursor
pagination реализованы. PostgreSQL/MySQL и полное выполнение
declarative admin actions ещё не готовы. `admin.surface.get` возвращает
read-only contract из `pluginprotocol`. Memory repository оставлен для
детерминированных smoke-тестов.

## Pagination cursor behavior

`forms.list` follows the request and response shapes in the versioned
`pluginprotocol` forms-db contracts. This plugin fixes the cursor behavior as
follows:

- Results use keyset order `createdAt DESC, id DESC`; the ID is the unique
  tie-breaker when timestamps match. The cursor points to the last item in the
  returned page, and the next page contains only rows strictly after that tuple
  in this order. The repository reads at most `limit + 1` matching rows to
  determine whether a next page exists.
- A cursor is bound to the exact `site`, `schemaName`, and equality filter.
  Reusing it with a different scope is rejected as `validation_failed`. A
  missing, malformed, modified, expired, or wrong-scope cursor has the same
  public error; responses and logs never include the token or its contents.
- Cursor lifetime is 15 minutes from issuance. Cursors are opaque authenticated
  tokens: encrypted claims include the scope digest, last `createdAt`/`id`
  tuple, issue time, and expiry. The token uses AES-256-GCM for claim privacy
  and HMAC-SHA-256 authentication; changing the token version, nonce, ciphertext,
  or authenticator invalidates it.
- The HMAC/encryption key is supplied by the deployment as an external secret
  file; the versioned cursor security contract defines the environment variable,
  encoding, and exact key length. The key is never part of `ConfigApply`, the
  forms settings, database rows, fixtures, API responses, or logs. Missing,
  unreadable, or incorrectly sized key material makes `forms.list` fail closed
  with the existing `storage_unavailable` response; other capabilities can
  continue operating.
- Keep the same secret mounted across process restarts and on every replica
  sharing a forms database. `ConfigApply` does not rotate the key. Rotate it by
  replacing the external secret and rolling all replicas; this intentionally
  invalidates outstanding cursors. The plugin does not retain old keys or offer
  a cursor migration window.

These details are plugin-owned implementation semantics recorded in the
versioned `internal/infrastructure/security/contracts/cursor.json` asset. The
public v1 JSON contract intentionally specifies cursor as an opaque optional
string and does not prescribe its encoding or cryptographic implementation.

## Локальная разработка

Репозиторий использует соседний checkout `pluginprotocol` через локальный
`replace` в `go.mod`; общий workspace задан в `plugins/go.work`. Сборка и тесты:

```bash
go build ./...
go vet ./...
go test ./...
```

Бинарник получает endpoint через `LIAPOLDUS_PLUGIN_ENDPOINT`; его нельзя
запускать на публичном listener.

Gateway smoke test (требуется соседний checkout Gateway):

```bash
LIAPOLDUS_CORE_ROOT="../../core" ./tests/gateway_smoke.sh
```

## Архитектура

```text
cmd/forms-db/                    корень композиции
internal/domain/models/          модель отправки формы
internal/domain/interfaces/      порт репозитория
internal/application/            сценарии работы с формами
internal/infrastructure/config/  разбор и проверка настроек
internal/infrastructure/contracts/ адаптер контрактов из pluginprotocol
internal/infrastructure/storage/ SQLite и детерминированное memory-хранилище
internal/presentation/plugin/    адаптер protocol и HTTP-envelope
tests/unit/                      unit- и boundary-тесты
tests/gateway_smoke.sh            интеграционная фикстура Gateway
```

Domain и application не импортируют protocol, Gateway или storage packages.
