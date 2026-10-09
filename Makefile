GOLANGCI_VERSION := v2.12.2
GOLANGCI := .tools/$(GOLANGCI_VERSION)/golangci-lint$(shell go env GOEXE)
export GOWORK := off
export GOFLAGS := -p=1
export GOTOOLCHAIN := go1.26.0

.PHONY: check lint lint-go generate check-generated check-race
$(GOLANGCI):
	GOBIN="$(CURDIR)/.tools/$(GOLANGCI_VERSION)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
lint-go: $(GOLANGCI)
	$(GOLANGCI) config verify --config .golangci.yml
	$(GOLANGCI) run --config .golangci.yml ./cmd/... ./contracts/... ./internal/... ./tests/... ./tools/...
lint: lint-go
	npm run lint
generate:
	go run ./tools/contracts
check-generated:
	go run ./tools/contracts --check
check: check-generated lint
	go test ./... -count=1
	npm test -- --maxWorkers=1
	go vet ./...
	go build ./...
check-race: check-generated
	go test -race ./... -count=1
