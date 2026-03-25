.PHONY: build run migrate import serve test clean docker-up docker-down

DSN ?= postgres://transit:transit@localhost:5432/transit?sslmode=disable

build:
	go build -o bin/tracker ./cmd/tracker

run: build
	./bin/tracker serve

migrate: build
	./bin/tracker migrate

import: build
	@test -n "$(GTFS)" || (echo "Usage: make import GTFS=path/to/gtfs.zip" && exit 1)
	./bin/tracker import --gtfs $(GTFS)

serve: build
	./bin/tracker serve

test:
	go test ./...

clean:
	rm -rf bin/

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f
