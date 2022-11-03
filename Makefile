GIT_COMMIT = '$(shell git rev-parse HEAD)'

build:
	go build -ldflags '-w -s -X shipyard/logging.gitCommit=$(GIT_COMMIT)'
