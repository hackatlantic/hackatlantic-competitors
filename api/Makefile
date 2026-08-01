SQLC_VERSION := 1.29.0

.PHONY: sqlc
sqlc:
	docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:$(SQLC_VERSION) generate
