# AGENTS.md — forms-db plugin

## Назначение

`plugins/forms-db/` — автономный product plugin простых форм. Он владеет
form schemas, submission records, продуктовым validation/filter/cursor
поведением и собственным storage adapter. Core управляет только generic
plugin instance/configuration/endpoint/policy; Server plugin обслуживает HTTP
и вызывает forms-db только по явно разрешённому plugin↔plugin контракту.

## Архитектурные границы

- Общий Core↔plugin lifecycle — только Plugin SDK REST с per-replica mTLS:
  Core уведомляет `Reload(generation)`, plugin запрашивает exact config
  generation сам, валидирует/применяет и подтверждает digest. Не получать
  settings из env/argv/application config files.
- После SDK миграции убрать из forms-db `pluginprotocol` lifecycle RPCs
  (`Bootstrap`, `Manifest`, `ConfigSchema`, `ConfigApply`, `Shutdown` и
  аналогичные control methods). Не сохранять fallback/compatibility API.
- `pluginprotocol` разрешён только как generic transport для вызовов/streams
  между plugins. Его код не знает `forms.*`, settings/schema, DB driver,
  cursor policy или forms errors. Все продуктовые contracts и payloads живут
  только в `contracts/v1/` этого plugin.
- Не добавлять forms-specific ветви, schema decoding или SQL/storage знание в
  Core, Plugin SDK или `pluginprotocol`.
- Constructor и `react-lib` заморожены. Не редактировать их код, зависимости,
  tests или TODO. Общие settings настраиваются Core API; plugin Admin Surface
  описывает только product-owned data/actions и не реализует настройки через
  lifecycle RPC.
- V1 запускается оператором вручную; plugin не устанавливает, запускает,
  рестартует, масштабирует и не удаляет workloads. Не добавлять TUF/catalog,
  Docker/Compose/Swarm/Kubernetes или CAPTCHA/Identity.

## Конфигурация, данные и secrets

- Plugin config — собственный JSON object и собственная строгая JSON Schema.
  Core сохраняет точные исходные bytes в `active`/`previous`; plugin их
  проверяет, не требует общей Core schema формы и не раскрывает значения.
- DSN/credentials задаются opaque references. Настоящее значение приходит
  только из scoped SDK grant после Reload, остаётся в памяти на время нужного
  generation и не попадает в конфигурацию, БД, logs/errors/metrics/audit.
- Product submissions и database state принадлежат forms-db, а не Core SQLite.
  Сохранять только документированные storage adapters. Не добавлять storage
  service для Core и не удалять существующий forms adapter без owner decision и
  migration/recovery evidence.
- Cursor secret выдаётся call-scoped grant; cursor bounded, integrity-protected
  и привязан к product query context. Не сохранять grant/cursor key или
  plaintext secret в долговременном storage.
- Изменение settings должно собирать и проверять candidate storage/schema до
  переключения; при ошибке прежние active settings и repository продолжают
  работать. Проверять concurrency и recovery настоящей SQL-backed конфигурации.

## Структура и качество

- Следуй четырёхслойной структуре существующего plugin; не добавляй новые
  layers для симметрии. Domain не зависит от Caddy/SQLite/MySQL/protocol.
- Contracts — source of truth; не дублировать error codes, schema defaults или
  JSON fields в нескольких слоях. SQL statements должны иметь очевидного owner
  и тестируемые migrations. Не выводить DSN, SQL arguments, secrets, cursor keys
  или request payloads в logs/errors.
- Tests держать под `tests/`, не помещать fixture или Go test code в
  production packages. Использовать существующие Go + Vitest runners.

## Процесс и проверки

- Перед работой проверить branch/HEAD/status/remotes и diff; сохранить всю
  user/agent WIP и generated state. Не угадывать remote.
- Делать законченный product vertical slice: contract → failing test → runtime
  implementation → child-process/conformance test. Не ослаблять уже зелёные
  gates ради реализации.
- Перед завершённым handoff выполнить `go test ./...`, `go build ./...`,
  `go vet ./...`, plugin TypeScript tests и Core/SDK child-process conformance
  для реально интегрированных путей. Если внешняя зависимость блокирует тест,
  перечислить это как OPEN.
- Обновлять `TODO.md` только фактическими остатками и проверками; no push,
  release, repo deletion или force operations.
