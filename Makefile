# Shared targets come from the locked installed kit package.
include node_modules/@polyspec/kit/scripts/kit/kit-bootstrap.mk

# The suite of the repository: the gates of kit and its own checks and tests.
CHECK_TARGETS := kit-check layout-check docs-check rules-check owner-validate rebuild-check language-check test-markers-check duplication-check parity-declarations-check parity-check link-check bypass-check suppression-check divergence-check lint-ts lint-php lint-go refs-check internal-paths-check test-go test-infrastructure

# The Composer of mise.toml, by its shim, which installs the PHP packages of the board example.
COMPOSER := $(KIT_TOOLS)/mise/shims/composer

# The Go of the checkout, by its absolute path (kit-check: a recipe starts a tool by its absolute path, as NODE and ALINT of kit.mk do).
GO := $(firstword $(wildcard $(KIT_TOOLS)/mise/shims/go.exe $(KIT_TOOLS)/mise/shims/go.cmd $(KIT_TOOLS)/mise/shims/go))

.PHONY: internal-paths-check
internal-paths-check: ## Check that no tracked file names a dependency's source or build output by its path (K1002); offline
	$(NODE) scripts/check-internal-paths.mts

.PHONY: refs-check
refs-check: ## Check that every task ID a document names resolves to a checklist item; offline
	$(NODE) scripts/check-refs.mts

# Kit owns native test discovery, workspace coverage and case-sensitive resources.
test-go: CASE_SENSITIVE_ENV := FLOWMARK_CASE_SENSITIVE_DIR

# The suites of flowmark, each runtime independently: the Node suite of the root infrastructure, and the Go workspace.
.PHONY: flowmark-node-check flowmark-go-check flowmark-test
flowmark-node-check: test-infrastructure ## Run the Node suite of the root infrastructure; offline
flowmark-go-check: build-go test-go ## Build and test the complete Go workspace; offline
flowmark-test: flowmark-node-check flowmark-go-check ## Run the Node and the Go suites (AGENTS: flowmark-test); offline

# The complete local install adds the locked board dependencies after the Kit bootstrap.
.PHONY: flowmark-install
flowmark-install: install-kit deps-php deps-node ## Install the Composer and npm packages of the board example from their locks; ONLINE

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

# Locked online dependency preparation: ci-setup calls one deps-<runtime> target per runtime of config/ci.json (K859), so
# the installs are owned here, each as the locked install of its package manager.
.PHONY: deps-go deps-node deps-php
deps-go: ## Download the locked Go workspace dependencies; ONLINE
	cd packages/go && $(ONLINE) $(GO) mod download

deps-node: ## Install the board example npm lock; ONLINE
	cd examples/board/web && $(ONLINE) $(NPM) ci

deps-php: ## Install the board example Composer lock; ONLINE
	$(ONLINE) $(COMPOSER) install --no-interaction --working-dir=examples/board/web

# The models of the board example are generated, never tracked (P34): the orm-gen of the installed @polyspec/orm and
# polyspec/orm writes them from the corpus catalog, and lint-ts and lint-php, which read them, write them first.
BOARD := examples/board/web
PHP := $(firstword $(wildcard $(KIT_TOOLS)/bin/php.cmd $(KIT_TOOLS)/bin/php))
.PHONY: board-models board-models-ts board-models-php
board-models: board-models-ts board-models-php ## Write the TypeScript and PHP models of the board example from the corpus catalog; offline

board-models-ts: ## Write node/model/models.ts of the board example with the orm-gen of its npm lock; offline
	cd $(BOARD) && $(NODE) node_modules/.bin/orm-gen gen --set ../block/dbspec/dbspec-set.json --out node/model --scan node/posts.ts

board-models-php: ## Write src/Model of the board example with the orm-gen of its Composer lock; offline
	cd $(BOARD) && $(PHP) vendor/bin/orm-gen gen --set ../block/dbspec/dbspec-set.json --out src/Model --namespace 'Flowmark\Board\Model'

lint-ts: board-models-ts
lint-php: board-models-php

# A parent benchmark runs through all of its sub-benchmarks: BenchmarkShardOpen took 31.9 s on an Apple M3 Pro, past the
# runner's 30 s default, so each test is given 120 s, about four times the slowest measured one.
.PHONY: flowmark-benchmarks
flowmark-benchmarks: ## Run the writepath tests and benchmarks and the statement benchmark through the Kit runner; offline
	$(NODE) $(KIT)/run-tests.$(KIT_EXT) go --timeout 120 --cwd packages/go -- -bench . -benchmem ./writepath
	$(NODE) $(KIT)/run-tests.$(KIT_EXT) go --timeout 600 --cwd packages/go -- -run '^$$' -bench BenchmarkStatement -benchtime 20x ./statement
