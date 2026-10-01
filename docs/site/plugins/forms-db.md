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
| `admin.surface.get` | unary | декларация страницы forms-db в Constructor |

## Business contract

Канонические JSON Schema requests/responses и mapping typed errors принадлежат
forms-db plugin и хранятся в его `contracts/v1/`. Общий plugin interface переносит
эти payloads как opaque JSON и не содержит forms-db contract.
Для публичной формы `site` задаёт доверенная маршрутизация Server-плагина, а не поле браузерского запроса.

### `forms.submit`

```json
{"site":"portal","schemaName":"contact","data":{"name":"Аня","email":"a@example.com"}}
```

`data` валидирует schema, зарегистрированная у instance, и ограничивается 256
полями по plugin payload contract. Неизвестный ключ даёт `validation_failed`.

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
плагина он получает тот же active config pull-запросом до readiness.

### `forms.list`

```json
{"site":"portal","schemaName":"contact","cursor":"optional","limit":50,"filter":{"field":"email","equals":"a@example.com"}}
```

Cursor — непрозрачное значение; плагин защищает его HMAC и использует для
keyset pagination. Результаты упорядочиваются по `createdAt DESC, id DESC`;
`id` обеспечивает стабильный порядок при совпадении времени. Cursor привязан к
точным значениям `site`, `schemaName` и equality-фильтра. Повторное применение
cursor с другим site, schema или фильтром отклоняется как `validation_failed`.
`filter` поддерживает точное JSON-equality по верхнеуровневому полю активной schema: учитываются
`properties`, `patternProperties`, локальные `$ref` и композиции schema.
Без `limit` используется 50; допустимый диапазон — 1–100. Нулевое,
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

Повторное удаление возвращает `not_found`; mapping в HTTP определяет контракт.

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
  "dsn": "secret://forms/sqlite-dsn",
  "cursorSecretRef": "secret://forms/cursor-key",
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

`admin.surface.get` возвращает декларативную страницу из единственного
plugin-owned `contracts/v1/admin-surface.json`; Go adapter читает её из
embedded assets этого плагина.

## Локальная проверка

Из каталога plugin-репозитория:

```bash
go build ./...
go vet ./...
go test ./...
```

## Страницы в Constructor

forms-db публикует одну declarative admin page через общий
[Plugin Admin Pages](/plugins/admin-pages) contract. Он не поставляет React
код и не открывает отдельный endpoint.

### Form submissions

Страница видна при `plugins.forms-db.read`. Верхняя filter form выбирает `site`
и `schemaName`, а также optional `field`/`equals`. Table вызывает `forms.list`
через Core `POST /api/plugins/{instance}/admin/pages/submissions/query`;
она показывает только `id`, `createdAt`, `data`, использует opaque cursor и
limit не выше 100. Значения `data` экранируются Constructor и never rendered
as HTML. `site` получает варианты из объявленного `optionsSource` capability
forms.list; `schemaName` запрашивает варианты тем же fixed query endpoint с
выбранным `site` как typed dependency. Constructor не превращает пустой select
в свободный ввод.

`Delete submission` вызывает `forms.delete` только для выбранной записи и
требует `plugins.forms-db.write`. Перед mutation Core возвращает
неисполняющий `428 confirmation_required` с одноразовым token; Constructor
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
