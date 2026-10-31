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
	golangci-lint run

# Scans the binary as released: build first, since a stripped build is what the
# gate in CI checks.
vuln: build
	./scripts/vuln-scan.sh bin/$(NAME)

# The hosted MCP server (shipyard mcp host): image and helm release. See
# docs/mcp.md, "Deploying the hosted server". Images are built from a release
# tag on a clean tree, so the tag names exactly what runs.
MCP_HOST_IMAGE ?= us-east4-docker.pkg.dev/shipyard-infra/shipyardbuild/mcp-host
MCP_HOST_CHART := helm/charts/mcp-host

check.mcp-host-tag:
	@git describe --exact-match --tags HEAD > /dev/null 2>&1 || { echo "Build the hosted MCP image from a release tag: HEAD is $(VERSION)"; exit 1; }
	@case "$(VERSION)" in *dirty*) echo "The tree has uncommitted changes ($(VERSION))"; exit 1 ;; esac

mcp-host.image: check.mcp-host-tag
	docker build --platform linux/amd64 --build-arg version=$(VERSION) --build-arg git_commit=$(GIT_COMMIT) \
	  -t $(MCP_HOST_IMAGE):$(VERSION) .

mcp-host.push: mcp-host.image
	docker push $(MCP_HOST_IMAGE):$(VERSION)

chart-test:
	./scripts/chart-test.sh

# MCP_HOST_TAG: the release tag to run. SHIPYARD_MCP_CLIENT_SECRET: this
# environment's client secret, from the secret store; it goes to helm on stdin,
# never on a command line. Use the current kube context for the right cluster.
define mcp_host_install
	@test -n "$(MCP_HOST_TAG)" || { echo "MCP_HOST_TAG is required, e.g. v1.11.0"; exit 1; }
	@test -n "$$SHIPYARD_MCP_CLIENT_SECRET" || { echo "SHIPYARD_MCP_CLIENT_SECRET is required"; exit 1; }
	@printf '%s' "$$SHIPYARD_MCP_CLIENT_SECRET" | grep -qE '^[A-Za-z0-9._~+/=-]+$$' || { echo "SHIPYARD_MCP_CLIENT_SECRET must be one token: letters, digits and ._~+/=-"; exit 1; }
	@printf 'mcp:\n  clientSecret: "%s"\n' "$$SHIPYARD_MCP_CLIENT_SECRET" | helm upgrade -i --atomic --timeout 5m \
	  --namespace $(1) --create-namespace \
	  --set-string image.tag=$(MCP_HOST_TAG) \
	  --set-string mcp.resourceUrl=$(2) \
	  --set-string mcp.siteUrl=$(3) \
	  -f - mcp-host $(MCP_HOST_CHART)
endef

mcp-host.install.production:
	$(call mcp_host_install,mcp-host,https://mcp.shipyard.build/mcp,https://shipyard.build)

mcp-host.install.qa:
	$(call mcp_host_install,qa-mcp-host,https://mcp.qa.shipyard.build/mcp,https://qa.shipyard.build)

.PHONY: check.mcp-host-tag mcp-host.image mcp-host.push chart-test mcp-host.install.production mcp-host.install.qa
