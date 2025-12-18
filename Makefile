.PHONY: lint

lint:
	./scripts/lint.sh

.PHONY: mappings-check

mappings-check:
	go run ./cmd/mappings-check -providers "$${MAPPINGS_CHECK_PROVIDERS:-aws}"

.PHONY: unmapped

# Generate/update unmapped metrics inventory (Option 1). Default: AWS S3.
unmapped:
	go run ./cmd/mappings-unmapped \
	  -provider aws \
	  -prefix s3 \
	  -products-root local/configs/products \
	  -mapping configs/mappings/s3.metrics.yaml \
	  -out configs/unmapped/s3.aws.yaml
