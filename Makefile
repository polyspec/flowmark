# The flowmark repository: the design phase holds documents only; the kit targets come from the vendored kit.mk.
include scripts/kit/kit.mk

.DEFAULT_GOAL := kit-test

.PHONY: refs-check
refs-check: ## Check that every task ID a document names resolves to a checklist item; offline
	node scripts/check-refs.mjs

.PHONY: test
test: ## Run the tests of the repository's own code and scripts; offline
	node --test tests/*.test.mjs
	node scripts/go-test.mjs

.PHONY: slug-vectors
slug-vectors: ## Record the heading ids GitHub and VS Code render into store/testdata/slug; needs the network, gh and npm
	node scripts/record-slugs.mjs

.PHONY: build
build: ## Build the executable that sidecar.json names
	go build -o build/flowmark ./cmd/flowmark
