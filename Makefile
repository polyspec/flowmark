# The flowmark repository: the design phase holds documents only; the shared targets come from the installed @polyspec/kit.
include scripts/kit/kit-bootstrap.mk

# The Go of the checkout, by its absolute path (kit-check: a recipe starts a tool by its absolute path, as NODE and ALINT of kit.mk do).
GO := $(CURDIR)/var/tools/mise/shims/go

.PHONY: refs-check
refs-check: ## Check that every task ID a document names resolves to a checklist item; offline
	$(NODE) scripts/check-refs.mts

# The repository's own tests. test, the task of kit, runs them as a prerequisite; the recipe of kit runs nothing more.
.PHONY: flowmark-test
flowmark-test: ## Run the tests of the repository's own code and scripts; offline
	$(NODE) --test tests/*.test.mts
	$(NODE) scripts/go-test.mts
# The macOS folder of the case-sensitive tests is mounted by hdiutil, which only macOS has, so its test runs only there.
ifeq ($(shell uname -s),Darwin)
	$(NODE) --test tests/darwin/*.test.mts
endif

test: flowmark-test

.PHONY: slug-vectors
slug-vectors: ## Record the heading ids GitHub, VS Code and GitLab render into store/testdata/slug; needs the network, gh, npm and cargo
	$(NODE) scripts/record-slugs.mts

# The check of the commit-msg hook: the message MESSAGE against the commit rules of kit, and the item rules of the index.
.PHONY: commit-msg-check
commit-msg-check: ## Check the commit message MESSAGE and the index against the commit rules of kit; offline
	$(NODE) $(KIT)/check-commits.mjs --message "$(MESSAGE)"

# The executable that sidecar.json names. build, the task of kit, builds it as a prerequisite.
.PHONY: flowmark-build
flowmark-build: ## Build the executable that sidecar.json names
	cd packages/go && $(GO) build -o $(CURDIR)/build/flowmark ./cmd/flowmark

build: flowmark-build
