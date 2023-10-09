.DEFAULT_GOAL := build

NAME := shipyard

VERSION = $(shell git describe --always --dirty --tags)
GIT_COMMIT = $(shell git rev-parse HEAD)

LDFLAGS = -X github.com/shipyard/shipyard-cli/version.GitCommit=$(GIT_COMMIT)
LDFLAGS += -X github.com/shipyard/shipyard-cli/version.Version=$(VERSION)
LDFLAGS += -s -w

build:
	go build -o bin/$(NAME) -ldflags "$(LDFLAGS)"

build-docker:
	docker build --build-arg version=$(VERSION) --build-arg git_commit=$(GIT_COMMIT) -t shipyard .

test:
	@go test ./... -cover
