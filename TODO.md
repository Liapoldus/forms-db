# TODO — forms-db

Здесь отслеживаются только незавершённые задачи плагина. Архитектура и JSON/wire
контракты принадлежат [pluginprotocol](../pluginprotocol); Gateway-интеграция и
общий план экосистемы описаны в [документации Liapoldus](../liapoldus.github.io).

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
- Админ-поверхность пока read-only: `admin.surface.get` публикует декларативный
  контракт, но изменяющие admin actions не исполняются.

## Осталось

- **Blocker: admin action invocation ещё не имеет полного protocol-контракта.**
  Единственный forms-db action в
  `pluginprotocol/contracts/forms-db/v1/admin-surface.json` — `delete`, который
  ссылается на `forms.delete` и помечен `dangerous: true`. Общая схема
  `pluginprotocol/contracts/admin-ui/v1/schema.json` задаёт Gateway URL
  `/api/plugins/{instance}/admin/pages/{page}/actions/{action}` и общую проверку
  capability, но не задаёт request/response schema, формат ошибок или точную
  семантику удаления для `forms.delete`. Текущий Go handler принимает
  `site`/`schemaName`/`id`, отвечает `deleted`/`id` или `not_found`, но это пока
  реализация без подтверждающего versioned contract; не считать её нормативной
  и не расширять, пока владелец `pluginprotocol` не закрепит payload и ответы.
  Нужны schema/vector/error contract и решение, должна ли повторная попытка
  удаления отсутствующей записи оставаться `not_found`.
- **Blocker: форма настроек admin surface ссылается на control RPC.** Страница
  `storage` использует `capability: "config.schema"`, однако `ConfigSchema` и
  `ConfigApply` — typed control RPC, а не capability в Manifest/Call. Общий
  admin UI contract не объясняет, как Gateway/Constructor должны отобразить
  такую страницу и отправить изменения. Не добавлять `config.schema` в Manifest
  и не превращать control RPC в `Call`, пока protocol contract не задаст явное
  отображение.
- Добавить в `pluginprotocol` недостающие нормативные admin-surface invocation
  contracts, после чего реализовать и протестировать подтверждённые actions,
  сохраняя Gateway-owned authorization/audit, plugin storage limits и
  redaction. До этого текущие `forms.list`/`forms.delete` runtime semantics не
  расширять и не обещать полную исполнимость admin UI.
- Расширить Gateway smoke так, чтобы он проверял SQL-backed работу с общей БД
  при нескольких plugin replicas и rotation внешнего cursor secret.
- Зафиксировать канонический Git remote для этого репозитория и выполнить push
  проверенных commits после подтверждения правильного URL.
