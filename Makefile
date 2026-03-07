MIGRATIONS_PATH=db/migrations

ifneq (,$(wildcard deploy/.env))
  include deploy/.env
  export
endif

ifneq (,$(wildcard .env))
  include .env
  export
endif

DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable
DB_URL_Docker=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@postgres:5432/$(POSTGRES_DB)?sslmode=disable

docker_run:
	docker run --rm \
		--name odoodevtools-server \
		--network odoodevtools_backend \
		-p 8080:8080 \
		-e DB_SOURCE=$(DB_URL_Docker) \
		-e GIN_MODE=release \
		odoodevtools:latest

new_migration:
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $(name)

migrate_up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose up $(steps)

migrate_down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose down $(steps)

migrate_goto:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose goto $(version)

migrate_force:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose force $(version)

sqlc:
	sqlc generate
generate:
	go generate

swagger:
	swag init -g cmd/server/main.go -d ./

.PHONY: sqlc docker_run new_migration migrate_up migrate_down migrate_goto migrate_force swagger generate