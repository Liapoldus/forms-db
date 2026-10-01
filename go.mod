module github.com/Liapoldus/forms-db

go 1.26.0

require (
	github.com/Liapoldus/plugin-sdk v0.0.0
	github.com/Liapoldus/pluginprotocol v1.1.0
	github.com/go-sql-driver/mysql v1.10.0
	github.com/jackc/pgx/v5 v5.8.0
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	modernc.org/sqlite v1.40.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/quic-go/quic-go v0.59.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/exp v0.0.0-20250620022241-b7579e27df2b // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	modernc.org/libc v1.66.10 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/Liapoldus/pluginprotocol => ../../pluginprotocol

replace github.com/Liapoldus/plugin-sdk => ../../plugin-sdk
