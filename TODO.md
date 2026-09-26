# TODO — forms-db

Здесь отслеживаются только незавершённые задачи плагина. Архитектура IPC и
общие JSON/wire-контракты принадлежат
[pluginprotocol](https://github.com/Liapoldus/pluginprotocol); актуальная
публичная страница продукта — [forms-db](https://liapoldus.github.io/plugins/forms-db).

Актуально на 2026-09-26. Локальный Git clone не имеет настроенного remote.

## Прогресс

- Аудит соответствия `pluginprotocol` v1 (2026-09-25): Manifest объявляет
  capabilities `forms.submit`, `forms.list`, `forms.delete` и
  `admin.surface.get`; для каждой возвращается ровно один descriptor с mode
  `CALL`. Admin surface загружается из единственного владельца контракта через
  `pluginprotocol.ContractFiles()` и совпадает с его fixture. Settings
  публикуются отдельным control RPC `ConfigSchema` и применяются через
  `ConfigApply`; `config.schema` в поле `storage.capability` admin surface —
  ссылка на этот control flow, а не capability в Manifest. Это различие не
  должно превращаться в специальную логику Gateway для forms-db. Имя продукта
  и `forms.*` capabilities являются ожидаемой идентичностью самого плагина;
  дополнительных Gateway-specific assumptions в его runtime boundary не
  обнаружено. Неоднозначность отображения control RPC в поле `capability`
  принадлежит общему admin-surface контракту `pluginprotocol`, а не этому
  репозиторию.
- Готовы адаптеры памяти, SQLite, PostgreSQL и MySQL. Интеграционное поведение
  SQL-пути проверено на PostgreSQL 17, MySQL 8.4 и MariaDB 11.4.
- Реализованы schema validation отправок, equality-фильтры и защищённая
  keyset-pagination с HMAC/AES cursor.
- HTTP response-action `body` сериализуется как UTF-8 JSON string согласно
  `pluginprotocol/contracts/http/v1/response-action.schema.json`; request body
  остаётся base64 согласно request contract. Unit test проверяет фактический
  JSON `CallResponse` через protocol decoder и не допускает base64 regression.
- Удалена загрузка cursor signing key через env и файл. Gateway выдаёт
  call-scoped `ActiveGrant` для `forms.list`; плагин обращается к
  `GrantBroker.RedeemGrant` из `pluginprotocol`, создаёт signer только на время
  одного вызова и очищает полученные байты. Bootstrap принимает только
  operational GrantBroker endpoint, settings/секреты в нём не передаются.
- SQL DSN теперь трактуется как opaque Gateway-issued reference в settings.
  `ConfigApply` обязан содержать revision-scoped `CONFIG_APPLY` grant с теми же
  instance ID, settings revision и secret reference. Плагин вызывает
  `GrantClient.RedeemConfig`, строит repository до атомарной замены активной
  конфигурации и не сохраняет DSN в settings после успешного применения.
  Неудачный grant или repository setup сохраняет предыдущую конфигурацию.
- Плагин принимает inherited loopback listener через `transport.ListenInherited`;
  production entrypoint не читает application settings, endpoint или cursor
  key из environment и не загружает application config files.
- Админ-поверхность пока read-only: `admin.surface.get` публикует декларативный
  контракт, но изменяющие admin actions не исполняются.

## Аудит мёртвого кода

- Go package graph включает composition root, protocol adapter, config,
  contracts, storage, security и application packages; их production-файлы
  имеют runtime или test consumers. Устаревший Gateway smoke script удалён:
  `rg` подтвердил отсутствие CI/build/code callers, а bootstrap schema отвергает
  создаваемые им `listeners/routes`; сценарий не доходил до plugin dispatch.
  Admin action handler остаётся, поскольку forms-db его явно объявляет, но не
  расширяется без подтверждённого versioned contract.
- Удалён неиспользуемый `NewServerWithCursorSigner`: у него не было call sites;
  lifecycle cursor signing теперь зависит от GrantBroker adapter-а, а не от
  заранее внедрённого долговечного signer-а.
- Для этого security boundary добавлены исполняемые Node TypeScript contract
  tests (`npm test`); Go unit/integration tests остаются основным поведением
  плагина.

## Осталось

- **Контракт admin action закрыт в pluginprotocol v1.** `forms.delete` принимает
  только `site`/`schemaName`/`id`, успешно отвечает `deleted`/`id`, а удаление
  отсутствующей записи (в том числе повторное) возвращает терминальную ошибку
  `not_found`/404. Ошибки валидации — `validation_failed`/422, а ошибки
  хранилища — повторяемая `storage_unavailable`/503. Plugin handler и SQL
  adapters должны сохранять это разделение; Gateway остаётся владельцем
  авторизации, audit и повторов.
- **Отображение настроек через control RPC закрыто в pluginprotocol v1.** Страница
  `storage` задаёт `ConfigSchema`/`ConfigApply` как control flow, а поля формы
  берутся из `ConfigSchema`; эти RPC не добавляются в Manifest и не вызываются
  как `Call` capability. Плагин уже предоставляет соответствующие RPC и
  protocol-owned admin surface.
- **Осталось: end-to-end admin action orchestration.** Plugin реализует
  `forms.delete`, однако полный Gateway/Constructor flow вызова declarative
  admin actions, проверок permission, audit и безопасной передачи результата
  должен быть подтверждён общей integration/conformance suite. Не дублировать
  его как специальную логику forms-db.
- Добавить актуальный Caddy-based Gateway child-process smoke для SQL-backed
  работы с общей БД при нескольких plugin replicas; проверить, что одинаковый
  Gateway-managed cursor secret через call-scoped grants валидирует cursor на
  другой replica, а его rotation инвалидирует старые cursors.
- Уточнить и проверить Gateway side: каждый SQL plugin instance получает только
  opaque DSN reference; `ConfigApply` выдаёт revision-scoped config grant, а
  отказ grant/repository setup не открывает candidate revision в dispatch.
- Установить и подтвердить канонический Git remote для этого репозитория, затем
  опубликовать проверенные локальные commits. Сейчас remote отсутствует; URL не
  угадывать.
