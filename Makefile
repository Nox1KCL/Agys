BINARY := agys
CMD := ./cmd/agys
BIN_DIR := ./bin

.PHONY: build run test fmt vet tidy install clean

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

tidy:
	go mod tidy

install: build
	./install.sh --local

clean:
	rm -rf $(BIN_DIR)
