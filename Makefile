.PHONY: run build test test-int lint lint-arch swagger migrate-up migrate-down migrate-create mocks up down seed

MIGRATE_DSN ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

run:
	go run ./adapters/rest

build:
	go build -o bin/tripmate-api ./adapters/rest

test:
	go test ./... -rapid.checks=1000

test-int:
	go test -tags=integration ./...

lint: lint-arch
	golangci-lint run

lint-arch:
	go run ./tools/archlint

swagger:
	go run github.com/swaggo/swag/cmd/swag@latest init -g adapters/rest/main.go -o docs

migrate-up:
	go run github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path migrations -database "$(MIGRATE_DSN)" up

migrate-down:
	go run github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path migrations -database "$(MIGRATE_DSN)" down 1

migrate-create:
	@test -n "$(name)" || (echo "usage: make migrate-create name=<name>" && exit 1)
	go run github.com/golang-migrate/migrate/v4/cmd/migrate@latest create -ext sql -dir migrations -seq $(name)

mocks:
	go run github.com/vektra/mockery/v2@latest

up:
	docker compose up -d postgres adminer

down:
	docker compose down

seed:
	@echo "No domain seed data exists in Sprint 00."
