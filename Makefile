# Shared targets come from the locked installed kit package.
include node_modules/@polyspec/kit/scripts/kit/kit-bootstrap.mk

# The suite of the repository: the gates of kit and its own checks and tests.
CHECK_TARGETS := kit-check toolchain-check layout-check docs-check rules-check owner-validate rebuild-check language-check test-markers-check duplication-check dependency-policy-check parity-declarations-check parity-check commits-check link-state-check bypass-check suppression-check divergence-check lint-ts lint-php lint-go refs-check flowmark-test

# The Composer of mise.toml, by its shim, which installs the PHP packages of the board example.
COMPOSER := $(KIT_TOOLS)/mise/shims/composer

# The Go of the checkout, by its absolute path (kit-check: a recipe starts a tool by its absolute path, as NODE and ALINT of kit.mk do).
GO := $(CURDIR)/var/tools/mise/shims/go

.PHONY: refs-check
refs-check: ## Check that every task ID a document names resolves to a checklist item; offline
	$(NODE) scripts/check-refs.mts

# Independent language suites let CI run without installing unrelated toolchains.
.PHONY: flowmark-test flowmark-node-check flowmark-go-check
flowmark-test: flowmark-node-check flowmark-go-check ## Run all repository tests; offline

flowmark-node-check: ## Run the Node infrastructure tests; offline
	$(NODE) --test tests/*.test.mts
ifeq ($(shell uname -s),Darwin)
	$(NODE) --test tests/darwin/*.test.mts
endif

flowmark-go-check: ## Run all Go modules with a case-sensitive folder; offline
	$(NODE) scripts/go-test.mts

test: flowmark-test

# The board example installs its Composer and npm packages, the polyspec ones as links to the sibling checkouts, from their
# locks; make install runs it after the bootstrap of kit (kit K246-2).
.PHONY: flowmark-install
flowmark-install: install-kit ## Install the Composer and npm packages of the board example from their locks; ONLINE
	$(ONLINE) $(COMPOSER) install --no-interaction --working-dir=examples/board/web
	cd examples/board/web && $(ONLINE) $(NPM) ci

install: flowmark-install

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
