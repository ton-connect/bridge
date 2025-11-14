.PHONY: generate clean tools FORCE

TOOLS_DIR := $(shell go env GOPATH)/bin
OAPI := $(TOOLS_DIR)/oapi-codegen
OAPI_PKG := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1
SPEC_URL := https://analytics.ton.org/swagger/doc.json
SPEC_RAW := $(CURDIR)/swagger-tonconnect.json
SPEC_BRIDGE := $(CURDIR)/swagger-tonconnect-bridge.json
CONFIG := $(CURDIR)/oapi-bridge.yaml
GEN_FILE := $(CURDIR)/bridge_events.gen.go

$(CURDIR):
	mkdir -p $@

$(SPEC_RAW): | $(CURDIR)
	curl -sS $(SPEC_URL) -o $@

FORCE:

$(SPEC_BRIDGE): $(SPEC_RAW) FORCE
	@command -v jq >/dev/null || (echo "jq is required" >&2 && exit 1)
	jq 'def bridge_defs: (.definitions | with_entries(select(.key | startswith("Bridge")))); \
	    def dummy_paths($$defs): (reduce ($$defs | keys[]) as $$name ({}; .["/dummy/\($$name)"] = {post: {requestBody: {content: {"application/json": {schema: {"$$ref": "#/components/schemas/\($$name)"}}}}, responses: {"204": {description: "stub"}}}})); \
	    bridge_defs as $$defs | \
	    {openapi: "3.0.0", info: {title: (.info.title // "TON Analytics"), version: (.info.version // "1.0.0")}, components: {schemas: $$defs}, paths: dummy_paths($$defs)}' $< > $@

$(OAPI):
	GO111MODULE=on go install $(OAPI_PKG)

tools: $(OAPI)

generate: tools $(SPEC_BRIDGE)
	$(OAPI) -generate models -config $(CONFIG) $(SPEC_BRIDGE)
	gofmt -w $(GEN_FILE)
