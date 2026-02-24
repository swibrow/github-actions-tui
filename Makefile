APP_NAME := gha
MODULE := github.com/swibrow/github-actions-tui

.PHONY: build install run clean fmt lint vet test tidy

build:
	go build -o $(APP_NAME) ./cmd/gha

install:
	go install ./cmd/gha

run: build
	./$(APP_NAME)

clean:
	rm -f $(APP_NAME)

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

vet:
	go vet ./...

test:
	go test ./...

tidy:
	go mod tidy
