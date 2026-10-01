# forms-db

Плагин простых форм Liapoldus v1. Оператор запускает отдельный процесс вручную.
Core хранит точные JSON-байты настроек и уведомляет плагин через Plugin SDK REST
`Reload`; плагин сам запрашивает указанное поколение, валидирует свой
[`settings.schema.json`](contracts/v1/settings.schema.json), строит candidate
repository и подтверждает поколение только после успешного переключения.
`pluginprotocol` используется исключительно для прямых вызовов других плагинов
к `forms.submit`, `forms.list`, `forms.delete` и `admin.surface.get`.

Поддерживаются SQLite, PostgreSQL и MySQL/MariaDB. `memory` оставлен для
детерминированных локальных тестов и не сохраняет записи. Product submissions
хранятся в выбранной БД плагина, не в Core SQLite. В `dsn` указывают только
opaque secret reference; реальный DSN получается по candidate-generation grant
через Plugin SDK. `cursorSecretRef` — отдельная ссылка для ключа подписи
пагинации: плагин запрашивает ключ на каждый `forms.list` и не хранит его в БД.
Versioned product contracts находятся в [`contracts/v1`](contracts/v1),
детали cursor — в
[`internal/infrastructure/security/contracts/cursor.json`](internal/infrastructure/security/contracts/cursor.json).

Оператор передаёт несекретные instance/replica IDs, Core URL и фиксированные
REST/peer endpoints через обязательные CLI-флаги. TLS identity и trust roots
задаются абсолютными путями к PEM/CRL; product settings через argv/env не
передаются. Core не запускает процесс и не управляет его restart в v1.
`--core-common-name`/`--core-uri` проверяют Core HTTPS server при config pull,
а `--core-client-common-name`/`--core-client-uri` отдельно проверяют Core
mTLS client на входящем `Reload`; один TLS identity для этих двух ролей не
предполагается.

```bash
GOWORK=off go build ./...
GOWORK=off go test ./...
GOWORK=off go vet ./...
npm test
```

`tests/integration/sdk-reload.test.ts` проверяет REST Reload → exact pull →
candidate grant → ACK в одном процессе. `child-process-sdk.test.ts` запускает
настоящий forms-db binary с mTLS, scoped DSN grant и generic peer submit,
затем рестартует его и проверяет сохранность SQLite записи. Это пока
contract-compatible Core fixture, а не полный тест с production Core.
Состояние остальных задач — в [`TODO.md`](TODO.md).
