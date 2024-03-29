FROM golang:1.22 AS build

WORKDIR /app

COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .

ARG version
ARG git_commit
ENV VERSION=$version
ENV GIT_COMMIT=$git_commit

# SY errors out obtaining VCS status: exit status 128
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /shipyard \
    -ldflags "-s -w -X github.com/shipyard/shipyard-cli/version.Version=${VERSION} -X github.com/shipyard/shipyard-cli/version.GitCommit=${GIT_COMMIT}"

FROM alpine:3.19
COPY --from=build /shipyard /usr/local/bin
