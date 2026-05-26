BIN := bin/pagepop
CMD := ./cmd/pagepop

.PHONY: all build run clean lint test fmt tidy

all: fmt tidy lint test build

build:
	go build -o $(BIN) $(CMD)

run: build
	$(BIN) --output $(or $(OUTPUT),./blog) --config $(or $(CONFIG),md_files.yml)

clean:
	rm -rf bin/

lint:
	go vet ./...

test:
	go test -v ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

deploy: build
	@echo "Deploy target not fully configured. Edit Makefile to specify deployment."
