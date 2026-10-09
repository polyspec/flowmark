# The flowmark repository: the design phase holds documents only; the kit targets come from the vendored kit.mk.
include scripts/kit/kit.mk

.DEFAULT_GOAL := kit-test

.PHONY: refs-check
refs-check: ## Check that every task ID a document names resolves to a checklist item; offline
	node scripts/check-refs.mjs

.PHONY: test
test: ## Run the tests of the repository's own code and scripts; offline
	node --test tests/*.test.mjs
	go test ./...

.PHONY: validate
validate: ## Validate the corpus: keys, references and the strict parse; offline
	go run ./cmd/flowmark validate .
