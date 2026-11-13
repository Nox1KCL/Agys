BINARY := agys
CMD := ./cmd/agys
BIN_DIR := ./bin

.PHONY: build run test fmt vet tidy install install-plugin clean

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

install-plugin:
	mkdir -p $(HOME)/.config/omarchy/plugins/nox.agys
	cp -R ./plugin/nox.agys/. $(HOME)/.config/omarchy/plugins/nox.agys/

clean:
	rm -rf $(BIN_DIR)
