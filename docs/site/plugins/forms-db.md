# forms-db

Плагин форм: приём и просмотр отправок веб-форм с memory-, SQLite-, PostgreSQL-
или MySQL-хранилищем. Эталонный пример плагина для
[гайда по созданию плагинов](/core/architecture/guide).

Репозиторий: **отдельный git-репозиторий** плагина — свой Go-модуль,
не в репозитории Core. Бинарник собирается из этого репозитория.
Core↔plugin configuration lifecycle обслуживает Plugin SDK REST; plugin-specific
settings и их schema остаются контрактом forms-db. `pluginprotocol` не
обслуживает Core lifecycle.

## Capabilities

| Capability | Тип | Назначение |
| --- | --- | --- |
| `forms.submit` | unary | сохранить отправку формы |
| `forms.list` | unary | список отправок |
| `forms.delete` | unary | удалить отправку |

## Business contract

Канонические JSON Schema requests/responses и mapping typed errors принадлежат
forms-db plugin и хранятся в его `contracts/v1/`. Общий plugin interface переносит
эти payloads как opaque JSON и не содержит forms-db contract.
Для публичной формы `site` задаёт доверенная маршрутизация Server-плагина, а не поле браузерского запроса.
Каждый capability payload должен содержать ровно один JSON object. Повторяющиеся
members на любом уровне, конфликтующие по регистру имена верхнего уровня и
дополнительный JSON document отклоняются как `validation_failed` (`422`);
исполняемые случаи перечислены в `contracts/v1/request-json-vectors.json`.

### `forms.submit`

```json
{"site":"portal","schemaName":"contact","data":{"name":"Аня","email":"a@example.com"}}
```

`data` валидирует schema, зарегистрированная у instance, и ограничивается 128
объявленными полями на form. Schema глубже 32 уровней отклоняется при применении
конфигурации. Настройки ограничены 256 KiB, а полный JSON request body
`forms.submit` — 1 MiB; превышение возвращает `validation_failed` до записи.
Неизвестный ключ даёт `validation_failed`.
Top-level request и успешный response формально описаны в
`contracts/v1/submit-request.schema.json` и
`contracts/v1/submit-response.schema.json`. Ответ содержит `frm_` ID,
`createdAt` в RFC 3339 и сохранённый объект `data`; неверная форма запроса или
несоответствие настроенной schema возвращает `validation_failed` (`422`), а
ошибка хранилища — только общий retryable `storage_unavailable` (`503`).
Примеры отказа проверяются contract-векторами и реальным peer handler в
`contracts/v1/submit-negative-vectors.json`.

Конфигурация instance передаёт `schemas` как объект, где ключ — имя схемы, а
значение — JSON Schema. Поддерживается Draft 2020-12; имя должно соответствовать
`^[a-z][a-z0-9_-]{0,63}$`. Core хранит исходный JSON object настроек в SQLite,
сохраняет его исходные bytes и валидирует документ generic-валидатором по
plugin-owned schema; Core не декодирует продуктовые поля. Плагин не получает
secret bytes или локальные file paths. Для SQL
DSN конфигурация содержит только opaque secret reference; Core выдаёт через
Plugin SDK REST instance/generation-scoped grant, по которому plugin отдельно
redeem-ит DSN у Core. DSN живёт только в памяти активной ревизии и
заменяется атомарно при успешном применении новой конфигурации. Внешняя загрузка
схем и удалённые `$ref` запрещены:
все используемые определения должны находиться внутри самой схемы (например,
в `$defs`). REST Reload сначала компилирует все схемы и готовит новое
хранилище, и только затем атомарно заменяет активную конфигурацию. При ошибке
активные настройки и хранилище остаются без изменений. Plugin держит применённую
revision в памяти. Core уведомляет плагин через REST `Reload(generation)`;
плагин сам запрашивает у Core именно эту immutable generation. После рестарта
плагина при работающем Core read-only readiness monitor отмечает его как
degraded, но не повторяет `Reload`. После проверки health оператор вручную
перезапускает Core; startup reconciliation передаёт active generation до того,
как replica снова станет ready.
На границе плагина settings должны быть ровно одним UTF-8 JSON object; повторные
имена members на любой глубине и любые trailing JSON documents отклоняются.
Plugin сначала проверяет объект по собственной versioned settings schema, затем
декодирует его в runtime-модель; ссылки `dsn` и `cursorSecretRef` остаются
непрозрачными для forms-db и проверяются только при соответствующей выдаче grant
Core.

### `forms.list`

```json
{"site":"portal","schemaName":"contact","cursor":"optional","limit":50,"filter":{"field":"email","equals":"a@example.com"}}
```

Cursor — непрозрачное значение; плагин защищает его HMAC и использует для
keyset pagination. Результаты упорядочиваются по `createdAt DESC, id DESC`;
`id` обеспечивает стабильный порядок при совпадении времени. Cursor привязан к
точным значениям `site`, `schemaName` и equality-фильтра. Повторное применение
cursor с другим site, schema или фильтром отклоняется как `validation_failed`.
Невалидированный или истёкший cursor отклоняется; корректный неистёкший cursor
не одноразовый: повторный запрос в том же scope повторно выполняет read-only
keyset query и не погашает cursor. Каждый вызов отдельно получает scoped grant
для ключа подписи. Эта повторяемость не гарантирует неизменный снимок данных:
между запросами строки могут быть добавлены или удалены.
`filter` поддерживает точное JSON-equality по верхнеуровневому полю активной schema: учитываются
`properties`, `patternProperties`, локальные `$ref` и композиции schema.
Без `limit` используется 50; допустимый диапазон — 1–100. Cursor ограничен
4 KiB фактически полученных байтов. SQL-вызовы ограничены timeout 5 секунд;
на instance допускается не более 64 одновременно выполняющихся или ожидающих
SQL операций. Нулевое,
отрицательное и превышающее максимум значение отклоняется. Неизвестное поле,
незарегистрированная schema или некорректный фильтр дают `validation_failed`
(`422`). Явное значение `null` отличается от отсутствующего поля. Cursor
ограничен по времени действия; при истечении, повреждении или несовпадении
scope плагин возвращает общий отказ без раскрытия содержимого cursor.
Срок действия cursor — 15 минут с момента выдачи.

Ключ подписи не входит в настройки как значение: `cursorSecretRef` указывает
на секрет Core, а plugin запрашивает scoped grant для `forms.list` через отдельный
Plugin SDK REST grant-redemption endpoint при обработке конкретного вызова.
Ключ не читается из env или локального файла и не сохраняется plugin-ом между
вызовами; он очищается после создания/использования signer-а. При отсутствии
или недоступности grant `forms.list` завершается fail-closed с
`storage_unavailable`; остальные capabilities могут продолжать работу. Все
replicas, работающие с общим хранилищем, получают один и тот же Core-owned
logical key через индивидуальные scoped grants. После ротации ключа ранее
выданные cursors становятся недействительными. Секрет не входит в REST `Reload`,
настройки форм, базу, ответы или логи.

### `forms.delete`

```json
{"site":"portal","schemaName":"contact","id":"frm_…"}
```

Повторное удаление возвращает `not_found` (`404`) через Core Admin Surface.
Core аудирует каждый принятый Admin Action отдельно, но не сохраняет его
request/response payload: email и ID записи не должны попадать в Core logs,
SQLite audit или ошибки. Forms-db не ведёт параллельный audit управления.

## Конфиг instance

Поддерживаются драйверы `memory`, `sqlite`, `postgres` и `mysql`. `memory` не
сохраняет записи после перезапуска и предназначен для локальной проверки.
Для persistent-драйверов DSN всегда получается по Core-scoped grant; в
settings допускается только opaque reference. MariaDB использует `mysql`.

Пример versioned JSON settings document, который forms-db запрашивает у Core
после `Reload` (это не локальный application-config файл plugin):

```json
{
  "driver": "sqlite",
  "dsn": "file:/absolute/path/to/forms-db-dsn",
  "cursorSecretRef": "file:/absolute/path/to/forms-cursor-key",
  "schemas": {"contact": {"type": "object", "properties": {"name": {"type": "string"}}}},
  "tablePrefix": "form_"
}
```

| Ключ | Назначение | По умолчанию |
| --- | --- | --- |
| `driver` | `memory`, `sqlite`, `postgres` или `mysql` | обязателен |
| `schemas` | именованные JSON Schema принимаемых форм | обязателен |
| `dsn` | opaque Core secret reference для любого persistent-драйвера; для `memory` запрещён | — |
| `cursorSecretRef` | opaque reference для подписи курсоров `forms.list` | — |
| `tablePrefix` | префикс таблиц плагина | `form_` |

Для SQLite значение секрета может быть абсолютным путём к plugin-owned файлу;
для PostgreSQL/MySQL — строкой подключения. Core выдаёт
instance/generation-scoped grant при конфигурировании, а plugin получает
реальный DSN отдельно и держит его в памяти активного поколения до его замены
или остановки.

Декларация плагина и привязка capability к маршруту — общий синтаксис
[«Обзор и настройка»](/plugins/).

Plugin SDK REST публикует декларативную страницу из единственного
plugin-owned `contracts/v1/admin-surface.json`. Core получает её по mTLS и
передаёт неизменённый документ клиенту управления. Запрос страницы
`submissions/query` отображается на plugin-owned capability `forms.list`, а
действие `submissions/delete` — на `forms.delete`; Core передаёт opaque page и
action ID через общий SDK JSON-action endpoint и не знает эти product strings.

## Локальная проверка

Из каталога plugin-репозитория:

```bash
go build ./...
go vet ./...
go test ./...
```

## Административная поверхность

forms-db публикует одну declarative admin page через общий
[Plugin Admin Pages](/plugins/admin-pages) contract и Plugin SDK REST. Он не
поставляет React код и не открывает отдельный management listener.

### Form submissions

В v1 Core разрешает чтение и mutation Admin Surface только service credential
`platform-admin`; доступ распространяется на весь forms-db instance. Отдельного
per-site allow-list/tenant ACL нет. `site` и `schemaName` — обязательные фильтры
запроса, но не граница авторизации. Значения `plugins.forms-db.read` и
`plugins.forms-db.write` в декларации являются отдельными Controller-side
permissions, если их проверяет UI backend; они не ограничивают Core/plugin
доступ по site.

Верхняя filter form выбирает `site` и `schemaName`, а также optional
`field`/`equals`. Table вызывает описанное в
Admin Surface действие `submissions/query` через Core Management API;
она показывает только `id`, `createdAt`, `data`, использует opaque cursor и
limit не выше 100. Значения `data` экранирует клиент управления и never rendered
as HTML. `site` получает варианты из объявленного `optionsSource` capability
forms.list; `schemaName` запрашивает варианты тем же fixed query endpoint с
выбранным `site` как typed dependency. Клиент не превращает пустой select
в свободный ввод.

`Delete submission` вызывает `forms.delete` только для выбранной записи и
в Controller может дополнительно требовать `plugins.forms-db.write`, но Core
в любом случае требует `platform-admin`. Перед mutation Core возвращает
неисполняющий `428 confirmation_required` с одноразовым token; клиент
показывает объявленное confirmation-сообщение и повторяет неизменный запрос
только после явного подтверждения. Оба запроса используют один
`Idempotency-Key`, а digest Surface передаётся через `If-Match`. Core
создаёт audit record с actor, instance, site, schemaName, record ID и outcome;
plugin получает минимальный typed input. Полный handshake описан в
[Plugin Admin Pages](/plugins/admin-pages).
Surface action объявляет `inputSchema` для `recordId` и `rowInput`
`{"recordId":"id"}`; поэтому UI передаёт ровно ID выбранной строки, а не весь
объект submission.

Настройки хранилища не являются отдельной capability или страницей плагина.
Их редактирование идёт через общий Core config API: raw JSON document без
обёртки, CAS-предусловие, новое поколение и REST `Reload` с точным pull.

Reference surface fixture: plugin-owned `contracts/v1/admin-surface.json`.

## Ручной запуск v1

Оператор запускает forms-db отдельным процессом после настройки Core bootstrap.
Все файловые пути TLS/CA/CRL обязательны и должны быть абсолютными. Product
settings, включая DSN, не передаются через командную строку или environment:

```bash
./forms-db \
  --instance-id=forms-db \
  --replica-id=forms-db-1 \
  --rest-listen=127.0.0.1:9544 \
  --core-url=https://127.0.0.1:9444 \
  --core-server-name=core.internal \
  --core-common-name=core-control \
  --core-client-common-name=core-control \
  --ca-file=/absolute/path/to/control-ca.pem \
  --server-cert=/absolute/path/to/forms-control.pem \
  --server-key=/absolute/path/to/forms-control-key.pem \
  --client-cert=/absolute/path/to/forms-client.pem \
  --client-key=/absolute/path/to/forms-client-key.pem \
  --crl-file=/absolute/path/to/control-crl.pem \
  --peer-listen=127.0.0.1:9643 \
  --peer-identity=spiffe://liapoldus.example/forms-db \
  --peer-allowed-caller=spiffe://liapoldus.example/server \
  --peer-ca-file=/absolute/path/to/peer-ca.pem \
  --peer-cert=/absolute/path/to/forms-peer.pem \
  --peer-key=/absolute/path/to/forms-peer-key.pem \
  --peer-carrier=tcp
```

## Направление развития в v3: website и редактор содержимого

Этот раздел фиксирует целевую продуктовую идею для v3, а не контракт текущей
версии. Вся дальнейшая работа над forms-db — включая replica/SQL compatibility,
rollout cohorts и website/content — отложена до v3. Это не меняет v1
capabilities, settings, Admin Surface, права
`platform-admin` или ручной запуск. До реализации v3 решение нужно уточнить по
вопросам в [`TODO.md`](https://github.com/Liapoldus/forms-db/blob/main/TODO.md).

V2 Core/SDK lifecycle и rollout не требуют изменений forms-db: generic
поведение проверяется на нейтральных fixtures, а существующий forms-db v1
остаётся regression target. Ранее выполненные локальные SQL replica tests
сохранены в owner TODO как evidence; они не означают поддержку shared multi-host
storage, schema-cohort rollout или production readiness. Новые product
совместимости и acceptance открываются только вместе с v3 owner планом.

В v3 forms-db может развиться из backend-а отправок форм в управляемый
плагином website-продукт: владелец задаёт структуру и данные сайта, плагин
сохраняет контент в поддерживаемом им хранилище, а автоматически
генерируемая админ-панель позволяет его просматривать и редактировать. Панель
должна поддерживать ролевую авторизацию и быть доступна на отдельно
настраиваемом сетевом endpoint/порту; она не должна случайно становиться
публичным маршрутом сайта. Источник метаданных для генерации UI и точная модель
ролей пока не утверждены.

Предварительно предполагаются такие обязанности forms-db:

- хранить контент сайта и связанные с ним продуктовые данные в собственном
  storage; не переносить эти данные в SQLite Core;
- валидировать и применять plugin-owned schema/config через общий lifecycle
  Plugin SDK; Core остаётся хранилищем точных config bytes и не интерпретирует
  сайт, роли, контент или DB-модель;
- предоставлять продуктовые операции чтения и изменения контента и описывать
  их собственными versioned contracts;
- предоставлять или собирать административный UI для этих операций, не
  добавляя UI-specific контракты в `pluginprotocol`; интерфейс должен
  генерироваться из явно утверждённых product metadata/schema, а не из
  произвольного introspection базы данных;
- использовать разрешённые межплагинные вызовы только через generic
  `pluginprotocol`; публикация/доставка публичного сайта и владение listener-ом
  должны быть согласованы с Server plugin, а не обходить его границу без
  отдельного решения.

В v3 потребуется описать и проверить границы доступа к панели, роли и их
назначение, разделение публичного контента и административных операций,
изменение схемы/миграции, публикацию и откат сайта, резервное копирование,
совместную работу с Server plugin, отказоустойчивость и безопасное обновление
контента. Нельзя считать текущий `platform-admin` policy достаточной моделью
для будущей website-админки: эта политика утверждена только для v1 Admin
Surface forms-db.

`--server-cert`/`--server-key` идентифицируют REST replica, ожидаемую Core в
`plugins[].replicas[].expectedPeerIdentity`. `--client-cert`/`--client-key`
используются для pull exact-generation config и secret grants; `--core-common-name`
и `--core-server-name` проверяют Core TLS server, а
`--core-client-common-name` — Core certificate для входящего `Reload`. URI SAN
проверяются дополнительно, если переданы `--core-uri` и `--core-client-uri`;
значение флага должно точно совпадать с URI SAN соответствующего
Core-сертификата. Это identity и trust для Core↔plugin REST.

В примере `forms-db`, `forms-db-1`, адреса и URI — демонстрационные значения;
они должны точно совпадать с Core bootstrap и Server peer target. Замените
placeholder paths существующими абсолютными путями.

Отдельная группа `--peer-*` включает plugin↔plugin listener; она использует
собственные CA и сертификат. `--peer-allowed-caller` должен точно совпадать с
Server `--peer-identity`; адрес из Server `--peer-endpoint` должен вести на
`--peer-listen`. При совместном запуске на одной машине используйте loopback;
при разнесении по private hosts listener должен быть доступен только от
разрешённых plugin peers. В v1 carrier — `tcp` или `quic`, но для базового
локального walkthrough применяется `tcp`.

После запуска Core replica inventory должен показывать forms-db как доступный,
а Server должен успешно выполнять разрешённые вызовы к нему. Если forms-db
перезапущен при работающем Core, inventory остаётся degraded до ручного
перезапуска Core и startup reconciliation; оператор не редактирует и не копирует
plugin settings в локальный файл.
