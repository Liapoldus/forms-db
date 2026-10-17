# Формы и service dispatch

Сервис forms-db вызывается напрямую сервисом Server по capability, разрешённой
владеющей services policy. Core хранит только generic settings и не хранит
service-to-service interaction policy в v1. Management API не проксирует
пользовательский payload.

Capability schemas и Admin Surface принадлежат forms-db service; Service SDK обслуживает общие REST lifecycle/admin endpoints, а `wool` — только прямую service-to-service сеть. См. [service Admin Pages](/services/admin-pages), [service configuration API](/core/api/config) и [целевую архитектуру](/core/architecture/target).
