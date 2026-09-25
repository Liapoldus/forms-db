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
- Ключ HMAC/шифрования поступает от окружения как внешний secret-файл. Имя
  environment variable, формат token и длина ключа заданы в версионированном
  security contract cursor. Ключ не входит в `ConfigApply`, настройки форм,
  строки БД, fixtures, ответы API или логи. Отсутствующий, недоступный или
  имеющий неверную длину ключ приводит к fail-closed ответу `storage_unavailable`
  для `forms.list`; остальные capabilities могут продолжать работу.
- Один и тот же secret должен оставаться смонтированным при перезапуске
  процесса и на каждой replica, использующей общее хранилище форм. `ConfigApply`
  не меняет ключ. Для ротации замените внешний secret и выполните rolling
  restart всех replicas; выпущенные ранее cursors намеренно станут
  недействительными. Плагин не хранит старые ключи и не предоставляет период
  миграции cursor.

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
```

Бинарник получает endpoint через `LIAPOLDUS_PLUGIN_ENDPOINT`; его нельзя
запускать на публичном listener.

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
