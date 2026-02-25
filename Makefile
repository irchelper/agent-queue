.PHONY: build test run vet clean clean-all

BIN := agent-queue

build:
	go build -o $(BIN) ./cmd/server

test:
	go test -race ./...

run:
	go run ./cmd/server

vet:
	go vet ./...

clean:
	rm -f $(BIN)

clean-all:
	rm -f $(BIN) data/queue.db
