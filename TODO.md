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

- Сверить admin actions с актуальным контрактом `pluginprotocol` и реализовать
  только объявленные им безопасные операции, сохраняя проверку actor/capability,
  audit/redaction и storage limits.
- Расширить Gateway smoke так, чтобы он проверял SQL-backed работу с общей БД
  при нескольких plugin replicas и rotation внешнего cursor secret.
- Зафиксировать канонический Git remote для этого репозитория и выполнить push
  проверенных commits после подтверждения правильного URL.
