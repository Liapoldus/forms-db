# TODO — forms-db v1

## Проверенное состояние на 2026-09-30

Последний локальный commit: `2d98319`. Product contracts, replica cursor tests и
Admin Surface обновлены, но lifecycle миграция не выполнена: текущая команда
`go test ./server/... ./forms-db/...` падает на импортах удалённых
`pluginprotocol/pluginv1` и `pluginprotocol/presentation/sdk` из forms-db
production/fixture packages. До сборки binary и Core→SDK→forms-db
Reload/pull/ACK smoke forms-db v1 не готов.

Этот файл содержит только задачи forms-db plugin. Единая граница версии и
межрепозиторный план: [`tasks/README.md`](../../tasks/README.md) и
[`tasks/prompts/form-plugin.md`](../../tasks/prompts/form-plugin.md). Product
settings, capabilities, storage semantics, errors и vectors принадлежат этому
репозиторию; lifecycle — [Plugin SDK](../../plugin-sdk/), peer transport —
[`pluginprotocol`](../../pluginprotocol/).

## Зафиксированная цель v1

- Отдельный вручную запускаемый plugin для обработки простых форм; никаких
  запусков/установок/рестартов со стороны Core.
- Core сохраняет exact plugin settings JSON bytes в `active`/`previous`,
  вызывает SDK REST `Reload(generation)`; plugin сам pull-ит named generation,
  проверяет schema, строит candidate storage/config и после атомарного apply
  ACK-ает digest.
- Plugin SDK REST — единственный Core↔plugin lifecycle. `pluginprotocol` может
  только передавать generic caller-defined plugin↔plugin messages/streams; он
  не содержит `forms.*`, ConfigSchema/ConfigApply, grants/settings lifecycle,
  manifests или формовых schemas.
- Core SQLite не хранит submissions. Form data и product-owned storage
  принадлежат forms-db; Core — plugin-agnostic и сохраняет только control-plane
  metadata/config bytes.
- Plugin settings, form schemas, validation, errors, query/filter/pagination,
  cursor format, CRUD actions и Admin Surface — только contracts этого repo.
- Constructor и `react-lib` заморожены; не менять их исходники/зависимости/tests.

## Зафиксированные product semantics

- Повторное удаление отсутствующей записи даёт терминальный `not_found`.
- Form values валидируются по активной plugin-owned schema до записи.
- Query pagination использует bounded keyset cursor, не offset scan; cursor не
  является авторизацией и не расширяет scope данных.
- Cursor signing secret выдаётся call-scoped; DSN выдаётся по
  instance/revision-scoped grant, раскрытое значение остаётся только в памяти
  активного runtime generation и очищается при смене поколения/остановке.
- Admin Surface предназначен для записей/операций plugin. Настройка общих
  settings не осуществляется устаревшей control RPC-формой `ConfigSchema` /
  `ConfigApply`; она идёт через generic Core settings API.
- Текущие storage adapters и migrations сначала инвентаризировать. Не считать
  PostgreSQL/S3 Core dependencies; не удалять формы-драйвер, пока не доказано,
  что он не входит в отдельный product scope.

## P0 — миграция Core lifecycle

- [ ] Перенести `cmd/forms-db` на Plugin SDK registration/bootstrap, health,
  readiness, `Reload`, exact config pull, digest ACK, metrics и structured logs.
- [ ] Добавить реальные per-replica mTLS listener/client identities и
  fail-closed verification. Не добавлять plaintext/bearer fallback.
- [ ] Перенести ConfigSchema/ConfigApply settings parsing/application с
  `pluginprotocol` в SDK plugin-owned schema/applier hooks; migrate runtime,
  tests, fixtures, generated references и go.work imports как единый breaking
  slice.
- [ ] Удалить gRPC Bootstrap/Manifest/ConfigSchema/ConfigApply/Shutdown service,
  code, assets и lifecycle tests после проверки отсутствия всех consumers.
- [ ] Прекратить чтение product config из env/argv/config files. При недоступном
  Core/grant не применять фиктивные default DB credentials.
- [ ] Обновить admin-surface contract и tests: удалить control-RPC config page;
  оставить product data/actions и безопасные scopes.

## P1 — config/repository lifecycle

- [ ] Строго проверить settings JSON/JSON Schema, unknown keys, size/depth,
  duplicate keys и secret-ref forms на границе Plugin SDK без потери raw bytes.
- [ ] Новый generation сначала создаёт candidate repository и компилирует все
  form schemas; только при полном успехе атомарно заменяет активную пару
  repository+schema+generation.
- [ ] На invalid DSN/grant/schema/migration/open failure сохранить старый
  repository и принимать запросы прежнего active generation до согласованного
  fencing поведения.
- [ ] Зафиксировать storage driver allow-list, transaction/connection pool
  defaults, schema/table prefix constraints, migration ownership, persistent
  volumes, backup/restore и startup recovery для каждого разрешённого driver.
- [ ] Установить limits для schema size/depth, form value/body size, field count,
  batch/read limits, filters, cursor length, database timeout/concurrency и
  admin page results; contract + executable boundary tests обязательны.
- [ ] Разделить storage errors: отсутствующая запись = `not_found`, transient
  outage = retryable service error, invalid data = validation error; не выдавать
  SQL driver message, query, DSN или path наружу.

## P2 — capabilities, grants и Admin Surface

- [ ] Проверить generic capability registration и route/action mapping без
  protocol-defined `forms.*` messages; объявить только реально реализованные
  forms methods в plugin-owned Manifest/schema.
- [ ] End-to-end установить caller→target policy для Server → forms-db в Core;
  plugin-to-plugin calls идут напрямую, Core не проксирует submission payload.
- [ ] Cursor grant: связать grant с replica/caller, invocation ID, site/schema/
  filter scope, bounded expiry и one-use. Проверить multi-replica continuation,
  invalid signature/version, key rotation и grant failure; не хранить signing
  keys в plugin DB.
- [ ] DSN grant: привязать к instance+config generation+purpose; выдать только
  до открытия candidate repository, не переиспользовать после нового
  generation, очищать in-memory secret при замене/закрытии.
- [ ] Admin actions: list, delete, pagination и operation/status; явная
  idempotency, page permission, delete confirmation, audit/redaction принадлежат
  соответствующим owners. Не добавлять отдельный plugin admin listener.
- [ ] Провести негативные проверки: cross-instance read/delete, wrong capability,
  revoked/stale grant, malicious filters/schema, oversized input, timing races,
  cursor tampering/replay и storage outage.

## P3 — tests и release quality

- [ ] Child-process SDK/mTLS conformance: startup, Reload/pull/ACK, invalid config
  сохранение прежнего state, operator restart, Core unavailable и recovery.
- [ ] Реальный SQL-backed integration: миграция, restart persistence, concurrent
  submit/list/delete, rollback candidate и отказ DB. Memory adapter не заменяет
  persistent acceptance.
- [ ] Межрепозиторный Server → protocol → forms-db dispatch: разрешённый вызов
  проходит напрямую; deny policy/mTLS failure не вызывает forms-db; Core API не
  видел user payload.
- [ ] Два процесса forms-db проверяют cursor continuation и independent
  readiness/generation fencing. Grants и records не утекли в logs/errors/metrics.
- [ ] Проверить plugin-owned contracts/vectors против runtime, Test fixtures не
  подменяют настоящий process и settings.
- [ ] Пройти `go test ./...`, `go build ./...`, `go vet ./...`, TypeScript suite,
  child-process security/integration, macOS/Linux build и совместную acceptance
  с Core/SDK/Server. Не фиксировать исторический PASS как актуальный.

## Не входит в v1

CAPTCHA, Identity/OIDC/OAuth, TUF и plugin install/update, управляемые Core
processes, Docker/Compose/Swarm/Kubernetes, Constructor/React UI changes,
protocol-defined forms methods, Core-specific storage/backends. Эти функции не
добавлять в contracts, schema, dependencies или binary.
