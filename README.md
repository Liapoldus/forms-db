# forms-db

Реализация плагина форм Liapoldus. Процесс объявляет capabilities
`forms.submit`, `forms.list`, `forms.delete` и `admin.surface.get`. `ConfigApply`
может выбрать постоянное хранилище SQLite; проверка отправок по переданным при
конфигурации JSON Schema Draft 2020-12 реализована. Схемы пока не сохраняются в
БД и должны приходить при каждом применении конфигурации. Equality-фильтры по
разрешённым верхнеуровневым полям активной schema и HMAC-protected cursor
pagination реализованы. Для постоянного хранилища поддерживаются SQLite,
PostgreSQL и MySQL; MySQL-совместимость проверена также на MariaDB. Полное
выполнение declarative admin actions ещё не готово. `admin.surface.get`
возвращает read-only contract из `pluginprotocol`. Memory repository оставлен
для детерминированных smoke-тестов.

## Семантика пагинации по cursor

`forms.list` следует формам запроса и ответа из версионированного forms-db
контракта `pluginprotocol`. Детали реализации cursor принадлежат этому плагину:

- Результаты упорядочены как `createdAt DESC, id DESC`. При одинаковом времени
  создания уникальный `id` служит tie-breaker. Cursor указывает на последний
  элемент выданной страницы; следующая страница содержит только записи строго
  после этой пары в указанном порядке. Репозиторий читает не более `limit + 1`
  подходящих записей, чтобы определить наличие следующей страницы.
- Cursor связан с точными значениями `site`, `schemaName` и equality-фильтра.
  Повторное использование в другом scope отклоняется с `validation_failed`.
  Отсутствующий, некорректный, изменённый, просроченный cursor и cursor для
  другого scope приводят к одному внешнему результату. Ответы и логи не
  содержат token или его внутренние claims.
- Срок действия cursor — 15 минут с момента выпуска. Claims зашифрованы и
  аутентифицированы: они включают digest scope, последнюю пару `createdAt`/`id`,
  время выпуска и истечения. Используются AES-256-GCM для конфиденциальности
  claims и HMAC-SHA-256 для аутентификации. Изменение версии token, nonce,
  ciphertext или authenticator делает cursor недействительным.
- Cursor key не передаётся через environment, файл, settings или `ConfigApply`.
  Для каждого `forms.list` Gateway передаёт request-scoped `ActiveGrant`; плагин
  получает ключ через protocol `GrantBroker.RedeemGrant`, создаёт signer только
  на время одного вызова и очищает полученные байты после обработки. Grant
  contract и его scope заданы в versioned security asset. Отсутствующий,
  недоступный или имеющий неверную длину ключ приводит к fail-closed ответу
  `storage_unavailable` для `forms.list`; остальные capabilities продолжают
  работу.
- Cursor signing key — Gateway-managed logical secret, общий для replicas,
  которым нужно валидировать cursors друг друга. Ротация выполняется заменой
  Gateway secret; все ранее выпущенные cursors намеренно становятся
  недействительными. Плагин не хранит старые ключи и не предоставляет период
  миграции cursor.
- Для SQL-хранилища `ConfigApply` содержит только opaque Gateway-issued
  reference для DSN. Gateway прикладывает revision-scoped `CONFIG_APPLY` grant,
  и плагин получает байты DSN через `GrantClient.RedeemConfig`; DSN остаётся в
  памяти storage adapter-а, но не сохраняется в активном объекте settings.
  Raw DSN не
  допускается в settings, протокольных логах, ответах или fixtures. Новый
  settings revision атомарно заменяет активный repository/DSN; неудачное
  применение сохраняет предыдущую активную конфигурацию.

Полная раскладка token, claim fields, единицы времени, key derivation и
криптографические параметры определены в versioned asset
[`internal/infrastructure/security/contracts/cursor.json`](internal/infrastructure/security/contracts/cursor.json).
Публичный JSON-контракт v1 намеренно задаёт cursor только как непрозрачную
необязательную строку и не закрепляет её кодирование или алгоритмы защиты.

## Локальная разработка

Репозиторий использует соседний checkout `pluginprotocol` через локальный
`replace` в `go.mod`; общий workspace задан в `plugins/go.work`. Сборка и тесты:

```bash
go build ./...
go vet ./...
go test ./...
npm test
```

Gateway передаёт плагину уже открытый loopback listener через inherited file
descriptor. Плагин не читает environment или application config files; при
старте Gateway выполняет typed `Bootstrap`, затем отправляет `ConfigApply` и
проверяет health до подключения плагина к dispatch.

Current Gateway child-process smoke is not yet available. The former script
generated a retired `listeners/routes` bootstrap document and did not validate
the current Caddy runtime; a replacement is tracked in `TODO.md`.

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
```

Domain и application не импортируют protocol, Gateway или storage packages.
