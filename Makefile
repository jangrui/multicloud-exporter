.PHONY: lint

lint:
	./scripts/lint.sh

.PHONY: mappings-check

mappings-check:
	go run ./cmd/mappings-check -providers "$${MAPPINGS_CHECK_PROVIDERS:-aws}"
