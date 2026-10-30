# Matches the go directive in go.mod: the image sets GOTOOLCHAIN=local, so an older Go fails
FROM golang:1.26 AS build

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

FROM alpine:3.22
COPY --from=build /shipyard /usr/local/bin

# The hosted MCP server holds its client secret and users' exchanged tokens;
# nothing in it needs root, and :8080 isn't a privileged port.
RUN adduser -D -u 10001 shipyard
USER 10001

# The hosted MCP server runs this image with: shipyard mcp host --listen :8080
EXPOSE 8080
