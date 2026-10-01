# TODO — forms-db v1

## Документация

- [x] Forms-db-owned Markdown/example перенесены в `docs/site/`; общие Core
  страницы принадлежат Core, сайт агрегирует owner docs по pinned SHA.
- [ ] После изменения owner docs обновить pin в
  `liapoldus.github.io/docs-sources.json` и проверить единый сайт.

## Проверенное состояние на 2026-09-30

Рабочее дерево содержит незакоммиченный переход на Plugin SDK: вручную
запускаемый binary, SDK REST `Reload`/exact pull/ACK, plugin-owned settings
schema, generic peer handlers и новые TypeScript integration fixtures.
`GOWORK=off go build ./...`, `go test ./...`, `go vet ./...` и `npm test`
прошли локально после добавления child-process fixture (10 Vitest файлов,
18 тестов и 2 Node contract tests). Новая fixture запускает
настоящий binary: mTLS Reload→exact pull→DSN grant→ACK, peer submit и
SQLite persistence после рестарта. Это ещё не production Core→SDK→forms-db
E2E; forms-db v1 пока не готов.

Этот файл содержит только задачи forms-db plugin. Единая граница версии и
межрепозиторный план: [`tasks/README.md`](../../tasks/README.md) и единый
[v1 prompt](../../tasks/CORE_V1_CODEX_SOL.md). Product settings, capabilities,
storage semantics, errors и vectors принадлежат этому
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
- Утверждённые v1 backends forms-db: SQLite, MySQL/MariaDB и PostgreSQL. Это
  product storage плагина, а не Core dependency. Для каждого backend проверить
  migrations, transactions, pool limits, persistence и backup/restore.

## P0 — миграция Core lifecycle

- [x] Перенести `cmd/forms-db` на Plugin SDK registration/bootstrap, health,
  readiness, `Reload`, exact config pull, digest ACK, metrics и structured logs.
- [ ] Добавить реальные per-replica mTLS listener/client identities и
  fail-closed verification. Не добавлять plaintext/bearer fallback.
- [x] Перенести ConfigSchema/ConfigApply settings parsing/application с
  `pluginprotocol` в SDK plugin-owned schema/applier hooks; migrate runtime,
  tests, fixtures, generated references и go.work imports как единый breaking
  slice.
- [ ] Удалить gRPC Bootstrap/Manifest/ConfigSchema/ConfigApply/Shutdown service,
  code, assets и lifecycle tests после проверки отсутствия всех consumers.
- [x] Прекратить чтение product config из env/argv/config files. При недоступном
  Core/grant не применять фиктивные default DB credentials.
- [x] Обновить admin-surface contract и tests: удалить control-RPC config page;
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
- [ ] Проверить allow-list SQLite, MySQL/MariaDB и PostgreSQL; закрепить
  transaction/connection pool defaults, schema/table prefix constraints,
  migration ownership, persistence, backup/restore и startup recovery для
  каждого backend. Другие drivers не поддерживать в v1. PostgreSQL repository
  contract прошёл на временном PostgreSQL 17 (2026-09-30); полный пакет тестов
  пока не собирается из-за старых lifecycle imports, а остальные пункты
  данного gate остаются открытыми.
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
- [ ] Реализовать caller→target allow/deny policy для Server → forms-db на
  стороне вызывающего plugin и применять её через generic `pluginprotocol`
  authorizer. Core не имеет peer-policy API в v1 и не проксирует payload;
  централизованное управление peer policy отложено до v2.
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

- [ ] Child-process SDK/mTLS conformance: startup, Reload/pull/ACK и operator
  restart c SQLite persistence покрыты `child-process-sdk.test.ts`; добавить
  invalid config с сохранением прежнего state, Core unavailable/recovery,
  revoked identity и настоящий production Core.
- [ ] Реальный SQL-backed integration: миграция, restart persistence, concurrent
  submit/list/delete, rollback candidate и отказ DB. Memory adapter не заменяет
  persistent acceptance.
- [ ] Межрепозиторный Server → protocol → forms-db dispatch: разрешённый
  HTTP→peer→SQL вызов проходит в child-process smoke с настоящими Server и
  forms-db (2026-10-01); отдельный вызывающий с доверенным CA, но неверной
  URI identity получает `unauthorized` до обработки submit. Ещё проверить
  отказ peer mTLS при недоверенном CA и доказать, что production Core API не
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
