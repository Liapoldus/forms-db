# forms-db

Runnable skeleton for the Liapoldus `forms-db` plugin.

The process owns the `forms.submit`, `forms.list`, `forms.delete` and
`admin.surface.get` capability names, but the persistence implementation is
intentionally a deterministic in-memory double. SQLite/PostgreSQL/MySQL
adapters are a later milestone.

## Local development

The parent `plugins/go.work` connects local development to the sibling
`pluginprotocol` checkout. A standalone clone uses the published module.
Build and test with:

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
internal/infrastructure/storage/ deterministic repository adapter
internal/presentation/plugin/    protocol and HTTP-envelope adapter
tests/unit/                      unit and boundary tests
tests/gateway_smoke.sh            Gateway integration fixture
```

Domain and application do not import protocol, Gateway or storage packages.
