# forms-db

Реализация плагина форм Liapoldus. Процесс объявляет capabilities
`forms.submit`, `forms.list`, `forms.delete` и `admin.surface.get`. `ConfigApply`
может выбрать постоянное хранилище SQLite; проверка отправок по переданным при
конфигурации JSON Schema Draft 2020-12 реализована. Схемы пока не сохраняются в
БД и должны приходить при каждом применении конфигурации. Equality-фильтры по
разрешённым верхнеуровневым полям активной schema поддерживаются.
HMAC-protected cursor pagination, PostgreSQL/MySQL и полное выполнение
declarative admin actions ещё не готовы. `admin.surface.get` возвращает
read-only contract из `pluginprotocol`. Memory repository оставлен для
детерминированных smoke-тестов.

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
