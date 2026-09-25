# TODO — forms-db

Здесь отслеживаются только незавершённые задачи плагина. Архитектура и JSON/wire
контракты принадлежат [pluginprotocol](../pluginprotocol); Gateway-интеграция и
общий план экосистемы описаны в [документации Liapoldus](../liapoldus.github.io).

## Прогресс

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
