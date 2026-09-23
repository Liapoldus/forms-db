# forms-db

Runnable skeleton for the Liapoldus `forms-db` plugin.

The process owns the `forms.submit`, `forms.list`, `forms.delete` and
`admin.surface.get` capability names, but the persistence implementation is
intentionally a deterministic in-memory double. SQLite/PostgreSQL/MySQL
adapters are a later milestone.

## Local development

The repository uses the sibling `pluginprotocol` checkout through the local
`replace` in `go.mod`. Build and test with:

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

