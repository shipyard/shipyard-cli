VERSION     = $(shell git describe --always --dirty --tags)
GIT_COMMIT = '$(shell git rev-parse HEAD)'

build:
	go build -ldflags '-w -s -X shipyard/version.GitCommit=$(GIT_COMMIT) -X shipyard/version.Version=$(VERSION)'
