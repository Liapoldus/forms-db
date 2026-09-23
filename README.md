# forms-db

Runnable implementation increment for the Liapoldus `forms-db` plugin.

The process owns the `forms.submit`, `forms.list`, `forms.delete` and
`admin.surface.get` capability names. `ConfigApply` can select a persistent
SQLite repository; PostgreSQL/MySQL adapters, schema-based validation,
contract-complete cursors/filters, and the declarative admin surface remain
unfinished. The memory repository remains available for deterministic smoke
tests only.

## Local development

The repository uses the sibling `pluginprotocol` checkout through the local
`replace` in `go.mod`; `plugins/go.work` provides the shared workspace. Build
and test with:

```bash
go build ./...
go vet ./...
go test ./...
```

The binary accepts its endpoint from `LIAPOLDUS_PLUGIN_ENDPOINT`; it must not
be started with a public listener.

Gateway smoke test (requires the sibling Gateway checkout):

```bash
LIAPOLDUS_CORE_ROOT="../../core" ./tests/gateway_smoke.sh
```

## Architecture

```text
cmd/forms-db/                    composition root
internal/domain/models/          Submission
internal/domain/interfaces/      Repository port
internal/application/            form use cases
internal/infrastructure/config/  settings parser and validation
internal/infrastructure/storage/ SQLite and deterministic memory repositories
internal/presentation/plugin/    protocol and HTTP-envelope adapter
tests/unit/                      unit and boundary tests
tests/gateway_smoke.sh            Gateway integration fixture
```

Domain and application do not import protocol, Gateway or storage packages.
