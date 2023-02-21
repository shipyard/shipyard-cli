FROM golang:1.20 AS build

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

# SY errors out obtaining VCS status: exit status 128
RUN CGO_ENABLED=0 go build -buildvcs=false -o /shipyard -ldflags "-s -w"

FROM alpine:3.17
COPY --from=build /shipyard /usr/local/bin
