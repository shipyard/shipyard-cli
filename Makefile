.DEFAULT_GOAL := build

NAME := shipyard

VERSION     = $(shell git describe --always --dirty --tags)
GIT_COMMIT = '$(shell git rev-parse HEAD)'

LDFLAGS  = -X shipyard/version.GitCommit=$(GIT_COMMIT)
LDFLAGS += -X shipyard/version.Version=$(VERSION)
LDFLAGS += -s -w

build:
	go build -ldflags "$(LDFLAGS)"

build-release-all:
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o "bin/$(NAME)-darwin-amd64"
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o "bin/$(NAME)-darwin-arm64"
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o "bin/$(NAME)-linux-amd64"
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o "bin/$(NAME)-linux-arm64"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o "bin/$(NAME)-win-amd64.exe"

clean:
	rm -rf bin
	rm $(NAME)

test:
	go test ./...
