# TODO — forms-db v1

## SQL/constants cleanup — 2026-10-09

Внутренние storage definitions перенесены из embedded
`internal/infrastructure/storage/contracts/storage.json` в typed Go constants
`storage_contract.go`; SQL templates/placeholders принадлежат
`sql_statements.go` того же пакета. Consumer audit охватил SQL/memory repositories,
генератор submission ID, TypeScript resource-limit test, build и документацию.
После миграции всех найденных потребителей удалены JSON asset, embed variable,
JSON loader/validation и per-repository contract field. Отдельных query/SQL files
в репозитории нет. Сохранены 12 исходных JSON values и 21 SQL template/placeholder,
включая существующий startup-lock WIP; новые product features не добавлялись.

Native `tests/unit/storage_definitions_test.go` проверяет ID format, default/max
list bounds для memory/SQLite, filtered keyset scan за пределами 101 строки,
durable SQLite names/index и sanitized errors. Existing native/child-process
tests продолжают проверять schema transaction rollback, scope, cursor и Reload.

Проверено с `GOWORK=off GOTOOLCHAIN=go1.26.0`: `go test ./... -count=1`,
`go build ./...`, `go vet ./...`,
`go test -race ./tests/unit ./tests/integration -count=1` — PASS.
`npm test -- --run --maxWorkers=1` — PASS: 22 Vitest files passed / 2 skipped,
35 tests passed / 9 skipped; Node cursor-secret tests 2/2.
`git diff --check` — PASS.

OPEN: `go test ./tests/integration -run 'Test(PostgreSQL|MySQL|MariaDB)RepositoryContract' -v -count=1`
пропускает все три backend gates без `FORMS_DB_{POSTGRES,MYSQL,MARIADB}_DSN`.
Девять внешних SQL child-process cases также skipped; migration-permission
checks требуют соответствующих `FORMS_DB_*_ADMIN_DSN`. Исторические SQL matrix
results ниже не являются повторной проверкой этого cleanup. Core/SDK release
cohort gates этим срезом не закрываются. Commit/push не выполнялись.

## Повторная проверка текущего WIP — 2026-10-06

PostgreSQL 16 red/green check обнаружил, что `sql-multi-replica-cursor.test.ts`
не проверял новый результат `cursorContinuedAcrossMixedGenerations`, уже
возвращаемый child-process fixture. Исправлено ожидаемое сообщение теста;
PostgreSQL cursor/fencing/mixed-generation child-process сценарий прошёл 1/1.
Полный локальный `npm test -- --run --maxWorkers=1` прошёл: 22 Vitest files,
35 passed / 9 skipped из-за отсутствующих SQL DSN; отдельные Node cursor tests
2/2. `GOWORK=off GOTOOLCHAIN=go1.26.0 go test ./...`, `go build ./...`,
`go vet ./...` и `git diff --check` прошли. В этом повторе MySQL и MariaDB
контейнеры не поднимались; их прежние результаты выше не перепроверялись.

## Историческая SQL multi-replica проверка — 2026-10-06

Повторная проверка текущего worktree 2026-10-06: focused
`sql-multi-replica-cursor` + `sql-multi-replica-startup` прошли 7/7 на
PostgreSQL 16, MySQL 8.0 и MariaDB 11.4. Полный `npm test -- --run
--maxWorkers=1` прошёл (23 Vitest files / 43 tests; отдельные Node cursor
tests 2/2); `GOWORK=off GOTOOLCHAIN=go1.26.0 go test ./...`, `go build
./...`, `go vet ./...` и `git diff --check` прошли. Использовались три
временных loopback-only Docker container с точными именами
`liapoldus-v2-sql-20261006-{pg,mysql,maria}`; после проверки контейнеры и
созданные ими anonymous data volumes удалены. Это повторяет DB conformance,
но не закрывает Plugin SDK/Core cohort compatibility gate ниже.

Исполняемый `tests/integration/sql-multi-replica-cursor.test.ts` прошёл на
PostgreSQL 16, MySQL 8.0 и MariaDB 11.4 (3/3) против отдельных временных Docker
containers. В каждом случае два настоящих forms-db child process использовали
общее SQL-хранилище: cursor первой replica продолжился на второй, а Reload
одной replica не изменил active generation другой. Полный `npm test` с теми же
DSN прошёл (22 файла / 39 Vitest tests и 2 Node tests); `GOWORK=off go test
./...`, `go build ./...`, `go vet ./...` и `git diff --check` также прошли.
Это подтверждает product-side SQL/cursor/fencing сценарий, но не SDK/Core
registration, lease или cohort rollout barrier; соответствующие пункты v2
остаются открытыми.

Параллельный cold-start двух независимых реплик проверяется в
`tests/integration/sql-multi-replica-startup.test.ts`: 12 раундов одновременно
инициализируют одинаковую схему и подтверждают read-after-write через соседний
process. Первый красный прогон воспроизвёл PostgreSQL catalog race на
`CREATE TABLE IF NOT EXISTS`. Теперь обе реплики сериализуют только startup DDL
DB-native advisory lock-ом на table prefix; runtime SQL не блокируется этой
защёлкой. Focused startup + cursor suite прошёл 7/7 на SQLite, PostgreSQL 16,
MySQL 8.0 и MariaDB 11.4, запущенных во временных контейнерах; SQLite fixture
также прошёл `go run -race`. Полный `npm test` прошёл: 21 файл / 34 теста,
9 SQL-backed tests пропущены без постоянных DSN; Node contract tests 2/2.
`GOWORK=off go test ./...`, `go build ./...`, `go vet ./...` и `git diff
--check` прошли. Этот тест проверяет cold start, но не release compatibility
или canary authorization.

Generic contract claims закреплены в локальном SDK v2 WIP: registration
экспортирует SemVer/digest `advertisedContracts` и `acceptedContracts` без
product-specific DTO; `application.ReleaseCohortCompatible` требует взаимного
acceptance claims между разными release digest и отказывает без evidence. Core
проверяет этот результат до promotion config generation. SDK `v1.0.0` и Core
опубликованного релиза всё ещё не содержат этот v2 контракт, поэтому изменения
пока нельзя считать интегрированными или публиковать.

Открыты: Core должен доказать на полном operation path, что отказ compatibility
gate оставляет active/previous pointers неизменными; регистрация новой
incarnation должна сериализоваться с promotion; release rollout/traffic cohort
gate ещё не завершён. После согласования и публикации SDK/Core contract forms-db
объявляет собственные settings-schema, durable SQL schema и peer API claims и
добавляет совместимые/несовместимые release fixtures с SQL matrix для
PostgreSQL, MySQL и MariaDB. forms-db не создаёт параллельный registration DTO,
endpoint или локальную имитацию Core fence.

## Проверка публикации — 2026-10-05

Коммит `f6dac5b` опубликован в `origin/main`; hosted Ubuntu verify прошёл.
После миграции на SDK `v1.0.0` и `pluginprotocol/v2 v2.0.0` полный local npm
suite прошёл (20 файлов passed / 1 skipped; 33 tests passed / 3 DB-dependent
skipped без DSN, Node cursor tests 2/2); `GOWORK=off` Go build/vet прошли.
Локальные SQL-backed проверки и production Core→Server→forms-db на трёх DB
прошли ранее и описаны ниже; hosted Core integration после этих commits ещё
выполняется.

## Повторная проверка — 2026-10-04

Текущий worktree прошёл `npm test -- --run` (20 файлов passed / 1 skipped,
33 теста passed / 3 SQL-dependent skipped), Node cursor tests (2/2),
`GOWORK=off go test ./...`, `go build ./...`, `go vet ./...` и
`git diff --check`. SQL-backed production Core→Server→forms-db E2E отдельно
прошёл 3/3 на одноразовых PostgreSQL 16, MySQL 8.0 и MariaDB 11.4. Новый
`sql-reload-concurrency` child process прошёл `go run -race` на macOS и в
Linux/arm64 Go 1.26 container. Владелец утвердил для v1 доступ `platform-admin`
ко всему forms-db instance; per-site allow-list/tenant policy не реализовывать.
Открытыми остаются hosted CI и docs pins. Linux runtime gate v1 прошёл в
OrbStack Ubuntu guest; отдельный bare-metal host не требуется.

Дополнение 2026-10-04: unit-test name для проверки SQL settings validation
переименован так, чтобы не смешивать Plugin SDK `Reload` lifecycle с удалённым
protocol RPC `ConfigApply`. Targeted `go test ./tests/unit -run
^TestConfigurationValidationAcceptsSQLAdaptersAndValidatesTablePrefix$` прошёл.
В cursor contract уточнено, что logical key для scoped grants управляется Core,
а ключ не входит в plugin settings; `cursor-runtime.test.ts` прошёл.
Те же `cursor-runtime.test.ts` и SQL configuration unit test прошли в OrbStack
Ubuntu 24.04.5 ARM64 (1/1 и 1/1).

Linux-проверка 2026-10-04: в Ubuntu 24.04.5 ARM64 VM под OrbStack прошли
TypeScript suite (20 файлов, 33 теста; один SQL-файл и три SQL-теста пропущены
без DSN), Node cursor tests (2/2), `go test ./...`, `go build ./...` и
`go vet ./...`. Отдельно на Linux ARM64 с Docker прошли forms-db SQL
child-process lifecycle tests для PostgreSQL 16, MySQL 8.4 и MariaDB 11.4
(3/3) с Core-compatible mTLS fixture. Это не production Core→Server→forms-db
SQL walkthrough: он проверен на macOS, а Linux VM полный сквозной walkthrough
прошёл на memory backend. Затем тот же production Core→Server→forms-db fixture
пройден в Linux VM на PostgreSQL 16, MySQL 8.0 и MariaDB 11.4 (3/3), включая
concurrency, candidate refusal/rollback, outage/recovery и restart persistence.
На дату этого snapshot-а hosted CI и published docs pins оставались открытыми;
оба gate прошли 2026-10-05. Per-site isolation явно исключена из v1 решением
владельца от 2026-10-04.

## Документация

- [x] Forms-db-owned Markdown/example перенесены в `docs/site/`; общие Core
  страницы принадлежат Core, сайт агрегирует owner docs по pinned SHA.
- [x] Owner docs опубликованы; pin
  `b25e654720bf5811da48a1d15f64f32bf55409a4` синхронизирован, VitePress
  build/deployment прошли.

## Проверенное состояние на 2026-10-02

Дополнение 2026-10-03: `npm test` прошёл (18 Vitest files / 31 tests;
один файл и три теста пропущены; отдельные Node cursor-grant tests 2/2),
`go test ./...`, `go build ./...` и `go vet ./...` прошли. Сквозной
Core→Server→forms-db отказ candidate и rollback прошёл на PostgreSQL 16,
MySQL 8.0 и MariaDB 11.4 (3/3).

Повторный текущий прогон `npm test -- --run` прошёл: 14 файлов / 25 тестов,
один файл skipped и 3 SQL cases skipped без DSN; отдельные Node cursor-grant
тесты прошли 2/2. SQL-backed disposable-container и production Core E2E ниже
ссылаются на отдельные прогоны с явно заданными DSN, это не следует трактовать
как покрытие обычного `npm test`.

`npm test` прошёл (14 Vitest files passed / 1 skipped; 25 tests passed / 3
SQL-backed cases skipped без DB DSN; плюс 2 Node cursor tests). `GOWORK=off go
test ./...`, `go build ./...`, `go vet ./...`, `git diff --check` и targeted
`gofmt` check прошли. При явных
runtime/admin DSN SQL child-process matrix отдельно прошла 3/3,
`GOWORK=off go test ./...`, `go build ./...`, `go vet ./...` и `git diff
--check` прошли. Дополнительно PostgreSQL 16, MySQL 8.0 и MariaDB 11.4
repository contract tests прошли против отдельных disposable containers; все
три DB repository contract tests реально исполнялись, не пропускались. Отдельно
real forms-db child-process lifecycle прошёл на PostgreSQL 16, MySQL 8.0 и
MariaDB 11.4: SDK REST/mTLS Reload, exact pull, scoped DSN grant, CRUD и restart
persistence. Child-process проверяет candidate SQLite open failure,
invalid-schema refusal, отказ SQL connection candidate на PostgreSQL/MySQL/
MariaDB без потери active данных и отказ trusted-CA caller с неверной URI
identity без сохранения payload. Эти низкоуровневые SQL process tests используют
mTLS Core fixture. Candidate DDL failure также прошёл на PostgreSQL 16, MySQL
8.0 и MariaDB 11.4: временная ограниченная DB identity успешно подключается,
но не может создавать таблицы; Reload отклоняется, active endpoint продолжает
отдавать прежние данные, временные identities удаляются после теста.
Теперь отдельный production Core→Server→forms-db E2E также проходит: Core
доставляет settings через SDK REST, а настоящий Server направляет HTTP
`forms.submit` напрямую в настоящий forms-db по разрешённому pluginprotocol
mTLS-вызову. Production Core→Server→forms-db SQL E2E прошёл на PostgreSQL 16,
MySQL 8.0 и MariaDB 11.4; последний локальный прогон прошёл 3/3 и дополнительно
проверил planned-downtime замену Core/plugin identities и всех trust roots:
новый CA сходится, старый отклоняется. Все три DSN включены в Core
cross-repository CI. Linux runtime, hosted CI и release/version gate остаются
открыты. Production readiness пока не заявляется.

Этот файл содержит только задачи forms-db plugin. Единая граница версии и
межрепозиторный план: [`tasks/README.md`](../../tasks/README.md). Product settings, capabilities,
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
- [x] Добавить реальные per-replica mTLS listener/client identities и
  fail-closed verification. Child-process SDK suite проверяет anonymous,
  wrong-identity, revoked и недоверенные certificates; plaintext/bearer
  fallback отсутствует.
- [x] Перенести ConfigSchema/ConfigApply settings parsing/application с
  `pluginprotocol` в SDK plugin-owned schema/applier hooks; migrate runtime,
  tests, fixtures, generated references и go.work imports как единый breaking
  slice.
- [x] Удалить legacy gRPC lifecycle service, assets и tests после перехода
  consumers на Plugin SDK REST; `pluginprotocol` оставлен только для generic
  peer communication.
- [x] Прекратить чтение product config из env/argv/config files. При недоступном
  Core/grant не применять фиктивные default DB credentials.
- [x] Обновить admin-surface contract и tests: удалить control-RPC config page;
  оставить product data/actions и безопасные scopes.

## P1 — config/repository lifecycle

- [x] Строгий settings decoder на runtime boundary: принимает только один UTF-8
  JSON object, отклоняет duplicate members на любой глубине и trailing JSON;
  unknown top-level keys отклоняются typed decoder-ом. Проверено настоящим Go
  settings runtime через `tests/contracts/strict-settings.test.ts`.
- [x] Settings schema выполняется перед typed runtime decode при SDK Reload;
  runtime дополнительно отвергает неоднозначный JSON, который стандартный
  `encoding/json` иначе принимает. Обязательные поля и schema constraints
  проверяются в `tests/contracts/settings.test.ts`, runtime decode — в
  `tests/contracts/strict-settings.test.ts`.
- [x] DSN/cursor references остаются opaque для forms-db: plugin не разбирает
  scheme/path и проверяет их пригодность только через scoped SDK grant. Фактическая
  Core redemption проверена отдельными lifecycle tests; продуктовый settings
  contract не дублирует Core secret-source format.
- [x] Новый generation сначала создаёт candidate repository и компилирует все
  form schemas; только при полном успехе атомарно заменяет активную пару
  repository+schema+generation. `tests/integration/sdk-applier.test.ts`
  проверяет успешную замену, refusal из candidate builder и сохранение прежней
  записи через active service после refusal.
- [x] Реальный forms-db process отклоняет candidate после успешного scoped DSN
  redemption, когда SQLite не может открыть candidate path; прежняя запись всё
  ещё доступна через активный peer endpoint. Evidence:
  `tests/integration/child-process-sdk.test.ts` и fixture `child-process`.
- [x] Child-process conformance доказывает, что invalid product schema и
  недоступный SQL endpoint в candidate settings не заменяют serving generation;
  сценарии выполнены для SQLite, PostgreSQL, MySQL и MariaDB с Plugin SDK mTLS
  fixture. Отдельный production Core E2E доказывает persistence/reconnect на
  всех трёх SQL backend.
- [x] Отсутствующий scoped DSN grant отклоняет candidate без изменения active
  generation и сохранения предыдущей записи; проверка выполняется в реальном
  forms-db child process под SDK mTLS fixture на SQLite, PostgreSQL, MySQL и
  MariaDB.
- [x] SQLite read-only candidate проходит соединение, но отказывает на DDL;
  живой forms-db process не подтверждает Reload и продолжает обслуживать active
  данные. Предусловия read-only/Ping/DDL failure проверяются в fixture.
- [x] Candidate migration/DDL failure сохраняет active generation на SQLite,
  PostgreSQL, MySQL и MariaDB. SQL process cases используют временного DB user с
  правом соединения, но без `CREATE`; после отказа candidate пользователь и его
  grants удаляются.
- [x] Проверить outage активной БД через production Core→SDK→Server→forms-db:
  во время недоступности SQL возвращается только retryable `storage_unavailable`
  без DSN/driver details, после восстановления сети сохранённая запись снова
  доступна. Реальный E2E прошёл на PostgreSQL 16, MySQL 8.0 и MariaDB 11.4.
- [x] Проверить allow-list SQLite, MySQL/MariaDB и PostgreSQL; закрепить
  transaction/connection pool defaults, schema/table prefix constraints,
  migration ownership и persistence для каждого backend. Другие drivers не
  поддерживать в v1. Repository contract tests прошли на disposable PostgreSQL
  16, MySQL 8.0 и MariaDB 11.4; срез 2026-10-02 дополнительно закрывает и
  повторно открывает repository и проверяет строки и scoped delete после reopen.
  Plugin-process restart и отказ DB в реальном Core→SDK→Server→forms-db
  lifecycle закрыты сквозным SQL E2E ниже. Core backup/restore относится к
  Core CLI acceptance и проверяется отдельно.
- [x] Зафиксировать и исполнять v1 resource limits в
  `contracts/v1/runtime-limits.json`: settings 256 KiB, schema depth 32,
  до 128 объявленных fields на form, submit request 1 MiB, page 100, cursor
  4 KiB, SQL timeout 5 s и до 64 одновременно ожидающих/выполняющихся SQL
  операций на instance. TypeScript запускает реальный Go runtime fixture на
  точной границе и на один байт/элемент выше для settings, schema depth,
  properties и submit body. SQL timeout проверяется отдельно с удерживаемой
  SQLite lock; concurrency cap реализован semaphore-ом и лимитом SQL pool.
  Остальные product-specific batch/filter/admin-result limits остаются
  ограничены соответствующими request contracts и не расширяются в этом slice.
- [x] Разделить публичные ошибки: отсутствующая запись = non-retryable
  `not_found` (`404`), invalid request/configured-schema data = `validation_failed`
  (`422`), storage outage = retryable `storage_unavailable` (`503`). Проверено
  delete Admin Action child-process, настоящим peer-handler через submit negative
  vectors и production Core→Server→forms-db SQL outage/recovery E2E; сообщения
  SQL driver, query, DSN и path наружу не возвращаются.

## P2 — capabilities, grants и Admin Surface

- [x] Проверить generic capability registration и route/action mapping без
  protocol-defined `forms.*` messages. Plugin-owned manifest перечисляет ровно
  реализованные `forms.submit`, `forms.list`, `forms.delete`; generic peer
  registry регистрирует те же методы, а Admin Surface связывает query/delete
  только с объявленными capabilities. Evidence: `tests/contracts/settings.test.ts`,
  `tests/contracts/admin-surface.test.ts`, request vectors и child-process
  submit/list/delete сценарии.
- [x] Применять caller identity allow-list на стороне forms-db через generic
  `pluginprotocol` authorizer. Настройка v1 задаётся оператором через
  `--peer-allowed-caller`; child-process suite доказывает успех разрешённого
  Server caller и отказ trusted-CA identity с другой URI до обработки payload.
  Core не имеет peer-policy API и не проксирует payload; централизованное
  управление peer policy отложено до v2.
- [x] Проверить multi-replica cursor continuation: cursor от первой отдельной
  forms-db process принимается второй process с тем же logical grant key и общей
  SQLite базой; исполняется в `tests/fixtures/multi-replica-cursor`.
- [x] Проверить cursor token runtime: query scope, expiry, key rotation,
  authenticated bytes и tampered version через
  `tests/contracts/cursor-runtime.test.ts` с настоящим Go signer.
- [x] Сквозная per-call выдача/redeem cursor grant проверена через
  Core→Server→forms.list/Admin Surface pagination: несколько cursor pages
  запрашиваются отдельно, а каждый page request получает cursor signer через
  SDK secret provider. Core audit содержит одно событие на запрос. Evidence:
  `core/tests/integration/manual-core-server.test.ts` и
  `core/tests/integration/manual-core-server-sql.test.ts` (PostgreSQL 16,
  MySQL 8.0, MariaDB 11.4, 3/3).
- [x] Сквозной stale/revoked identity отказ для cursor grant на
  Core→Server→forms.list: active generation 3, plugin остаётся на generation 2
  при недоступном Reload endpoint; затем Core CRL отзывает plugin client
  certificate, и реальный scoped-grant request из `forms.list` не проходит
  mTLS. Server возвращает только `503 storage_unavailable`, после возврата
  trust policy Core/plugin сходятся и `forms.list` возвращает 200. Проверено
  `core/tests/integration/manual-core-server.test.ts` (1/1) и
  `core/tests/integration/manual-core-server-sql.test.ts` (3/3 на PostgreSQL 16,
  MySQL 8.0, MariaDB 11.4). Это отказ отозванной workload identity на grant
  boundary; отдельного API отзыва конкретного уже выданного handle в v1 нет.
- [x] Проверить отказ неподдерживаемой capability для mTLS-аутентифицированного
  разрешённого caller-а: `forms.unknown` даёт `ErrMethodNotFound`, marker
  payload не сохраняется, последующий authorized list возвращает пустой набор.
  Проверено реальным child process в
  `tests/integration/child-process-sdk.test.ts`.
- [x] Доверенная по CA, но не allow-listed plugin identity отвергается до
  обработки `forms.list` и `forms.delete`, как и `forms.submit`; проверяется
  настоящий child process и generic peer authorizer в
  `tests/integration/child-process-sdk.test.ts`.
- [x] Проверить реальную SQL-backed конкуренцию между invocation и config
  replacement: SQLite child process блокирует `Adapter.Use`, применяет candidate
  через `Adapter.Apply`, подтверждает, что Reload ждёт старый SQL-вызов, прежний
  repository закрывается только после него, данные прежнего поколения сохраняются,
  а новые вызовы обслуживаются отдельной candidate-базой. Проверено
  `tests/integration/sql-reload-concurrency.test.ts` и отдельно
  `GOWORK=off GOTOOLCHAIN=go1.26.0 go run -race
  ./tests/fixtures/sql-reload-concurrency`.
- [x] Не добавлять tenant/site policy в v1: владелец выбрал общий
  `platform-admin` доступ ко всему instance. `site` — фильтр данных, а не ACL;
  Admin Surface `permissions` остаются независимой Controller-side политикой.
- [x] Остальные перечисленные негативные пути уже имеют runtime evidence:
  malformed schema/request отказ, лимит oversized submission, отказ
  tampered cursor (`peer-list-cursor.test.ts`), storage outage/recovery и
  stale/revoked replica grant в Core→Server→forms-db SQL walkthrough. Каждый
  cursor page request получает новый scoped grant.
- [x] SQL-подобное неподдерживаемое имя filter field отклоняется до repository
  query с `422 validation_failed`; проверено generic peer handler-ом реального
  fixture в `tests/integration/peer-list-cursor.test.ts`.
- [x] Зафиксирована повторяемость валидного cursor: тот же scope в пределах
  TTL повторяет read-only keyset query без погашения cursor; каждый вызов заново
  получает scoped key grant. Fixture проверяет одинаковую страницу на
  неизменном наборе через две replicas (`peer-list-cursor.test.ts`).
- [x] Runtime исполняет replica vectors `old-cursor-after-key-rotation` и
  `cursor-grant-unavailable-on-one-replica`: настоящий peer handler возвращает
  соответственно `422` после смены ключа и `503` при отказе scoped grant;
  TypeScript сравнивает фактические коды с `cursor-replica-vectors.json`.
- [x] Параллельный ConfigApply не закрывает repository, пока выполняется
  invocation, начатый на старом поколении; после завершения вызова candidate
  атомарно становится обслуживающим. Доказано через настоящий
  `restplugin.Adapter` в `tests/fixtures/reload-concurrency` и
  `tests/integration/reload-concurrency.test.ts`; `go run -race` тоже прошёл.
- [x] Актуальный Core→Server→forms-db ручной memory walkthrough повторно прошёл
  в Linux/arm64 Go 1.26 container 2026-10-04; submit/list/Admin Surface, cursor
  grant, restart/reconnect, revocation и redaction подтвердились.
  generic Core grant tests уже покрывают replica, active generation, purpose,
  one-use redemption и redaction. Child-process SDK fixture подтверждает
  отдельный grant на каждый forms.list и отказ повторного redemption уже
  потраченного handle по mTLS. Signing keys не хранятся в plugin DB.
- [x] DSN grant lifecycle: Core связывает выдачу с replica, exact active
  generation, configured reference и purpose и допускает одно redemption;
  forms-db redeem-ит secret при подготовке candidate repository, вызывает
  `Destroy`, очищает временный DSN и не сохраняет plaintext в active settings.
  Отказ grant/connection/schema не меняет serving generation. Проверено
  `core/tests/integration/plugin-secret-grants.test.ts`,
  `tests/integration/child-process-sdk.test.ts` и SQL child-process cases.
- [x] Реализовать product Admin Surface для list/pagination/delete без отдельного
  plugin admin listener. Страница требует read+write permissions, delete имеет
  подтверждение и row mapping; повторное удаление отсутствующей записи явно
  возвращает non-retryable `not_found`. Runtime покрыт
  `tests/integration/child-process-sdk.test.ts` и
  `tests/integration/admin-surface-sdk.test.ts`; management authorization/audit
  остаются ответственностью Core и Plugin SDK.
- [x] Подтвердить end-to-end Core audit/redaction повторённых Admin Actions:
  manual Core→Server→forms-db E2E дважды выполняет query и delete через Core,
  проверяет результат каждого вызова, по одной audit-записи на каждый принятый
  запрос и отсутствие email/record ID в Core logs, SQLite и audit fields. Core
  audit остаётся единственным аудитом управления; forms-db его не дублирует.
  Evidence: `core/tests/integration/manual-core-server.test.ts`.
- [x] Проверить production Core mTLS secret grant после смены generation:
  grant, выданный для generation 1, отклоняется после Reload generation 2 как
  `grant_denied`; handle отсутствует в ответе, Core logs, SQLite и audit.
  Production Core→Server→forms-db сценарий прошёл на PostgreSQL 16, MySQL 8.0
  и MariaDB 11.4 (`core/tests/integration/manual-core-server-sql.test.ts`, 3/3).
- [x] Закрепить семантику idempotency/retry для синхронных Admin Actions:
  `Idempotency-Key` служит только корреляции/audit и не дедуплицирует вызовы;
  каждый принятый повтор заново выполняет action и создаёт отдельную audit
  запись. Повторный `forms.delete` возвращает `not_found`, а не сохранённый
  первый ответ. Это не exactly-once; неизвестный результат не replay-ится
  автоматически. Durable operations и artifact actions сохраняют отдельную
  operation-idempotency семантику Core. Описание — в
  `core/docs/site/core/api/operations.md`; поведение проверяется manual E2E.
- [x] Отдельная повторная negative-matrix запись поглощена сводным пунктом выше;
  не поддерживать дублирующий список сценариев.

## P3 — tests и release quality

- [x] Строгий decoder для capability payload требует ровно один JSON object и
  отклоняет дублирующиеся members, case-fold collision верхнего уровня и
  trailing document как `validation_failed`. Contract-векторы исполняются
  реальным forms-db peer handler в `tests/integration/peer-list-cursor.test.ts`.
  Generic Server HTTP envelope может содержать opaque cookie context; forms-db
  извлекает только `body`, не интерпретируя и не сохраняя context values.
  Проверено child fixture с cookie marker в
  `tests/integration/peer-dispatch.test.ts` и production Core→Server→forms-db
  E2E в `core/tests/integration/manual-core-server.test.ts`; SQL-backed variant
  также утверждает payload/cookie redaction на PostgreSQL, MySQL и MariaDB
  (`core/tests/integration/manual-core-server-sql.test.ts`, 3/3).
- [x] Child-process SDK/mTLS conformance: startup, Reload/pull/ACK, operator
  restart и SQLite persistence покрыты `child-process-sdk.test.ts`; invalid
  schema и недоступный candidate SQL connection не меняют active generation
  на четырёх storage-вариантах. Отсутствующий scoped DSN grant также отказан
  без изменения active generation на всех четырёх child-process backends.
  SQLite read-only DDL failure и SQL DDL denial на PostgreSQL, MySQL и MariaDB
  также сохраняют active generation.
- [x] Дополнить production Core E2E rollback после отказа forms-db candidate.
  `core/tests/integration/manual-core-server-sql.test.ts` 2026-10-03 прошёл на
  PostgreSQL 16, MySQL 8.0 и MariaDB 11.4 (3/3): недоступный candidate
  получает терминальный failed operation, прежний runtime продолжает принимать
  submissions, после rollback восстанавливаются submit/list и точные settings
  bytes. Исправлен typed-nil SQL repository при ошибке builder-а; регрессия
  закреплена в `tests/integration/sdk-applier.test.ts`.
- [x] SQL-backed child-process lifecycle на PostgreSQL, MySQL и MariaDB:
  миграция, Reload/pull/scoped DSN grant, persistent CRUD и restart persistence.
  Выполнено против disposable DB containers; настоящее production Core в этом
  наборе заменяет mTLS test fixture.
- [x] Проверить конкуренцию на реальных SQL repository: одновременные
  submit/list, затем delete/list и финальную keyset-пагинацию без потерь и
  повторов. Набор прошёл на PostgreSQL 16, MySQL 8.0 и MariaDB 11.4.
- [x] Проверить cursor continuation между двумя независимыми forms-db adapters:
  первая runtime выдаёт cursor, вторая runtime с тем же Core-owned logical key
  продолжает страницу на общем repository; tampering по-прежнему отклоняется.
  Это доказывает replica-independent token semantics, но не заменяет отдельный
  process/mTLS/readiness/generation-fencing test ниже.
- [x] Сквозная concurrency-проверка Core→SDK→Server→forms-db выполняет 24
  параллельных submit и bounded cursor pagination на PostgreSQL 16, MySQL 8.0 и
  MariaDB 11.4; тот же сценарий проверяет persistence, ручной restart,
  reconnect, DB outage и recovery. `tests/integration/manual-core-server-sql.test.ts`
  прошёл 3/3 и после добавления planned-downtime trust-root rotation.
- [x] Дополнить production Core→SDK→Server→forms-db SQL путь проверкой
  candidate-generation refusal/rollback при storage connection failure.
  Сквозной production Core путь прошёл на PostgreSQL 16, MySQL 8.0 и MariaDB
  11.4 2026-10-03. Отдельные child-process tests по-прежнему проверяют отказ
  candidate на SQLite, PostgreSQL, MySQL и MariaDB, включая DDL denial.
- [x] Межрепозиторный Server → protocol → forms-db dispatch: разрешённый
  HTTP→peer→SQL вызов проходит в child-process smoke с настоящими Server и
  forms-db; forms-db child-process тест доказывает отказ trusted-CA identity с
  неверной URI и отсутствие сохранённого submission payload.
- [x] Child-process peer mTLS отвергает вызов с сертификатом от недоверенного
  CA до обработки; после попытки проверяется отсутствие submission в storage.
  Проверено `tests/integration/child-process-sdk.test.ts`.
- [x] Production Core→Server→forms-db child-process walkthrough подтвердил,
  что marker прямого `forms.submit` отсутствует в Core stdout/stderr, durable
  SQLite/WAL и сохранённых audit-полях: `core/tests/integration/manual-core-server.test.ts`.
- [x] Два отдельных forms-db процесса проходят Plugin SDK mTLS health/readiness,
  разделяют SQLite storage и per-call logical cursor key; успешный cursor
  continuation не требует sticky routing. Одна replica применяет generation 2,
  вторая остаётся ready на generation 1; их runtime-schema ответы подтверждают
  independent generation fencing. Исполняется в
  `tests/integration/multi-replica-cursor.test.ts`.
- [x] Child-process conformance проверяет, что маркеры submitted/unauthorized
  records, DSN и secret references отсутствуют в stdout/stderr настоящего
  процесса, ошибке отказа unauthorized peer и ответе Prometheus metrics
  endpoint. Проверка ограничена этими lifecycle/request путями; при добавлении
  новых error/logging paths их нужно включать в ту же redaction suite.
- [x] Негативные `forms.delete` vectors исполняются настоящим peer handler:
  missing `id`, пустой `site` и неизвестное поле дают `422 validation_failed`.
  `peer-dispatch.test.ts` передаёт versioned vectors в реальную Go fixture и
  проверяет runtime result.
- [x] Runtime-conformance inventory versioned vectors закрыта: request JSON,
  submit-negative и delete-negative vectors выполняются через настоящий
  child-process plugin с загруженными settings/SDK lifecycle; Admin Surface
  mapping vector проходит как SDK Admin Action; cursor/replica vectors
  проверяются runtime-handler и двумя независимыми mTLS processes. Schema-only
  проверки остаются отдельной проверкой формы контракта, не подменяя runtime.
- [x] На 2026-10-04 повторно пройти `go test ./...`, `go build ./...`,
  `go vet ./...`, TypeScript suite и child-process security/integration в
  OrbStack Ubuntu guest; SQL E2E проверить на PostgreSQL, MySQL и MariaDB. Все
  перечисленные проверки прошли; hosted CI и Core cross-repository integration
  также прошли. Tag `v1.0.1` создан после gates. Per-site policy явно не входит
  в v1 по решению владельца.

## Не входит в v1

CAPTCHA, Identity/OIDC/OAuth, Core-managed plugin install/update или process
management, Docker/Compose/Swarm/Kubernetes,
protocol-defined forms methods, Core-specific storage/backends. Эти функции не
добавлять в contracts, schema, dependencies или binary.

## V3 — replica/SQL compatibility и website/content

В v2 `plugins/forms-db/` не изменяется: текущая v1 simple-forms функция служит
только regression baseline, а generic Core/SDK rollout проверяется на fixtures.
Нижеследующие ранее записанные v2 задачи перенесены в v3. Выполненные проверки
сохранены как технические evidence; они не подтверждают поддержку multi-host
storage, release-cohort compatibility или production readiness.

- [ ] Перейти на Plugin SDK self-registration/lease и versioned peer-directory;
  объявлять SemVer/digest, совместимость config schema, storage schema и
  product peer contracts. Gate: новая incarnation, loss/return replica,
  Core restart и двухкогортный rolling/canary без replay.
- [x] Проверить выдачу cursor между двумя независимыми child-process replicas
  на общем MySQL, MariaDB и PostgreSQL backend; проверить независимый generation
  fence при изменении активной формы. Исполняемый gate:
  `tests/integration/sql-multi-replica-cursor.test.ts`.
- [x] Проверить одновременный cold-start двух процессов на одной схеме и
  read-after-write между ними на SQLite, PostgreSQL, MySQL и MariaDB; 12
  параллельных открытий на backend проходят в
  `tests/integration/sql-multi-replica-startup.test.ts`.
- [x] `tests/integration/mixed-generation-cursor.test.ts` проверяет настоящий
  child-process сценарий: подписанный cursor продолжается на обеих SQL replicas
  после Reload только первой; вторая остаётся на предыдущем generation.
  Для того же fixture вручную прогнаны отдельные temporary MySQL 8.0, MariaDB
  11.4 и PostgreSQL 16 services; каждый child-process run подтвердил
  `firstReplicaAdvanced`, `secondReplicaRemainedOnPreviousGeneration`,
  `cursorContinuedAcrossMixedGenerations` и process generation fence. Команда:
  `FORMS_MULTI_REPLICA_SQL_DRIVER=<mysql|postgres> FORMS_MULTI_REPLICA_SQL_DSN=<test-dsn> GOWORK=off GOTOOLCHAIN=go1.26.0 go run ./tests/fixtures/multi-replica-cursor`.
  Это подтверждает forms-db cursor semantics в смешанных generations, но не
  release-cohort compatibility или Core activation barrier. Эти локальные SQL
  services удалены после теста; DSN и test credentials не сохранялись.
- [ ] После публикации общего SDK/Core owner contract объявлять product-owned
  ranges для settings schema, durable SQL schema и peer contract; до разрешения
  canary Core обязан сверять compatibility всех живых release cohorts и
  блокировать несовместимый rollout. Текущий точный контрактный blocker и
  требуемые поля описаны в начале этого файла; forms-db не создаёт конкурирующую
  регистрацию и не имитирует Core fence.
- [ ] Потреблять только явные peer links и transport endpoint, выбранный SDK
  resolver; не переносить Core policy в формы. Gate: same-placement socket,
  remote TCP/QUIC и отсутствие скрытого fallback.

Проверку release artifacts, установку и плановые обновления во всех версиях
выполняет оператор выбранными средствами. Core не
управляет workload или количеством replicas и не получает provider API.

### Website и редактирование контента

Целевая идея и границы владельцев описаны в [документации forms-db](docs/site/plugins/forms-db.md#направление-развития-в-v3-website-и-редактор-содержимого)
и агрегированы в [Core roadmap](https://liapoldus.github.io/core/architecture/v1-migration-roadmap#v3-forms-db-website-и-управление-контентом). Это отдельная
работа после v2 rollout; не добавлять её в v1/v2 contracts, acceptance, зависимости
или binary.

- [ ] Спроектировать forms-db как plugin-owned website/content service поверх
  текущего storage ownership; определить связь content model с существующими
  forms и site identifiers.
- [ ] Спроектировать административный UI и отдельный endpoint/port: bind
  address, TLS/mTLS или иная внешняя граница, сетевые ACL, публикация за Server
  plugin или прямой listener, защита от случайной публичной экспозиции.
- [ ] Определить principal/role lifecycle, роли и permissions, provisioning,
  credential/session model, recovery и audit; не считать v1
  v1 `platform-admin`/Admin Surface готовым решением.
- [ ] Выбрать декларативный источник/формат, из которого генерируется admin UI;
  ограничить отображаемые и изменяемые поля явной product schema/metadata,
  определить версионирование и безопасный fallback при несовместимых схемах.
- [ ] Определить формат сайта и разделение публичных и административных
  маршрутов; решить, кто обслуживает публичный сайт — Server plugin или
  forms-db — и как передаются immutable releases, `current`/`previous` и
  rollback.
- [ ] Описать versioned settings, content/action contracts, schema evolution,
  migrations и concurrency/CAS semantics; Core остаётся plugin-agnostic и
  принимает только generic settings schema.
- [ ] Определить lifecycle сохранения/публикации контента, backup/restore,
  retention, limits, failure recovery и поведение при недоступности storage.
- [ ] Добавить security threat model, abuse/resource limits, redaction/audit
  правила и негативные conformance scenarios до реализации.
- [ ] Проверить сквозной путь Core → Plugin SDK → forms-db и интеграцию с Server
  без изменения `pluginprotocol` в сторону product-specific методов.

### Решения, требуемые до реализации v3

- [ ] Подтвердить, является ли website/content режим расширением `forms-db` или
  отдельным plugin product; не смешивать его с отправками форм без явного
  ownership contract.
- [ ] Выбрать владельца публичного HTTP listener-а и site release lifecycle:
  Server plugin, forms-db либо согласованный split между ними.
- [ ] Уточнить, означает ли «отдельный порт» отдельный listener самого forms-db
  или отдельный listener Server plugin, и какие сетевые источники могут его
  достигать.
- [ ] Выбрать authN/authZ модель админ-панели, способ первичного provisioning и
  требуемые роли/права.
- [ ] Уточнить, какие виды контента входят в v3 и должны ли формы/отправки
  использовать ту же content DB и модель разрешений.
- [ ] Утвердить обязательные storage backends для website content и требования
  к миграциям/совместному использованию DB с текущими формами.
