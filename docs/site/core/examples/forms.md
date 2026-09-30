# Формы и plugin dispatch

Плагин forms-db вызывается напрямую Server plugin-ом по capability, разрешённой
владеющей plugins policy. Core хранит только generic settings и не хранит
plugin-to-plugin interaction policy в v1. Management API не проксирует
пользовательский payload.

Capability schemas и Admin Surface принадлежат forms-db plugin; Plugin SDK обслуживает общие REST lifecycle/admin endpoints, а `pluginprotocol` — только прямую plugin-to-plugin сеть. См. [plugin Admin Pages](/plugins/admin-pages), [plugin configuration API](/core/api/config) и [целевую архитектуру](/core/architecture/target).
