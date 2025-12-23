.PHONY: lint

lint:
	./scripts/lint.sh

.PHONY: mappings-check

mappings-check:
	go run ./cmd/mappings-check -providers "$${MAPPINGS_CHECK_PROVIDERS:-aliyun,tencent,aws}"

.PHONY: check
check: lint
	go test -v -race -cover ./...
