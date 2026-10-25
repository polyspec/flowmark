# The flowmark repository: the shared targets come from the kit checkout beside it, whose kit-bootstrap.mk includes its kit.mk
# and runs its sources as $(KIT)/<tool>.$(KIT_EXT) (kit K614, K613).
include ../kit/scripts/kit/kit-bootstrap.mk

# The suite of the repository: the gates of kit and its own checks and tests.
CHECK_TARGETS := kit-check toolchain-check layout-check docs-check rules-check owner-validate rebuild-check language-check test-markers-check duplication-check dependency-policy-check parity-declarations-check parity-check commits-check link-state-check bypass-check suppression-check divergence-check lint-ts lint-php lint-go refs-check flowmark-test

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
	$(NODE) $(KIT)/check-commits.$(KIT_EXT) --message "$(MESSAGE)"

# The executable that sidecar.json names. build, the task of kit, builds it as a prerequisite.
.PHONY: flowmark-build
flowmark-build: ## Build the executable that sidecar.json names
	cd packages/go && $(GO) build -o $(CURDIR)/build/flowmark ./cmd/flowmark

build: flowmark-build
