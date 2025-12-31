.PHONY: lint

lint:
	./scripts/lint.sh

.PHONY: mappings-check

mappings-check:
	go run ./cmd/mappings-check

.PHONY: check
check: lint
	go test -v -race -cover ./...
