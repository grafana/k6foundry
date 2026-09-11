# Run the k6-ci golangci-lint config locally. See grafana/k6-ci/README.md.
# Targets: lint, update-lint-patch, clean-lint.
# Overrides: WORKFLOW, LINT_BASE, LINT_FINAL, LINT_PATCH.
#
# Produces two gitignored files at the repo root:
#   .golangci-base.yml  cached download from grafana/k6-ci (only re-fetched
#                       when WORKFLOW changes)
#   .golangci.yml       effective config = base + LINT_PATCH (if present)

WORKFLOW   ?= .github/workflows/test-lint.yml
K6_CI_REF  := $(shell grep -oE 'grafana/k6-ci/[^@[:space:]]+@[A-Za-z0-9._/-]+' $(WORKFLOW) | head -n1 | cut -d@ -f2)
BASE_URL   := https://raw.githubusercontent.com/grafana/k6-ci/$(K6_CI_REF)/.golangci.yml

LINT_BASE  ?= .golangci-base.yml
LINT_FINAL ?= .golangci.yml
LINT_PATCH ?= .golangci.patch

$(LINT_BASE): $(WORKFLOW)
	curl -fsSL $(BASE_URL) -o $@

$(LINT_FINAL): $(LINT_BASE) $(wildcard $(LINT_PATCH))
	cp $(LINT_BASE) $@
	@if [ -f $(LINT_PATCH) ]; then \
	  echo "Applying $(LINT_PATCH)"; \
	  git apply $(LINT_PATCH); \
	fi

.PHONY: lint
lint: $(LINT_FINAL)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$$(head -n1 $(LINT_BASE) | tr -d '# ') \
	  run --config=$(LINT_FINAL) ./...

.PHONY: update-lint-patch
update-lint-patch: $(LINT_BASE)
	@if [ ! -f $(LINT_FINAL) ]; then \
	  echo "Run 'make lint' first to materialize $(LINT_FINAL), edit it, then re-run."; \
	  exit 1; \
	fi
	-diff -u --label a/.golangci.yml --label b/.golangci.yml $(LINT_BASE) $(LINT_FINAL) > $(LINT_PATCH)

.PHONY: clean-lint
clean-lint:
	rm -f $(LINT_BASE) $(LINT_FINAL)
