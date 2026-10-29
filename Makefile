# Shared targets come from the locked installed kit package.
include node_modules/@polyspec/kit/scripts/kit/kit-bootstrap.mk

# The suite of the repository: the gates of kit and its own checks and tests.
CHECK_TARGETS := kit-check toolchain-check layout-check docs-check rules-check owner-validate rebuild-check language-check test-markers-check duplication-check dependency-policy-check parity-declarations-check parity-check commits-check link-state-check bypass-check suppression-check divergence-check lint-ts lint-php lint-go refs-check test-go test-infrastructure

# The Composer of mise.toml, by its shim, which installs the PHP packages of the board example.
COMPOSER := $(KIT_TOOLS)/mise/shims/composer

# The Go of the checkout, by its absolute path (kit-check: a recipe starts a tool by its absolute path, as NODE and ALINT of kit.mk do).
GO := $(firstword $(wildcard $(KIT_TOOLS)/mise/shims/go.exe $(KIT_TOOLS)/mise/shims/go.cmd $(KIT_TOOLS)/mise/shims/go))

.PHONY: refs-check
refs-check: ## Check that every task ID a document names resolves to a checklist item; offline
	$(NODE) scripts/check-refs.mts

# Kit owns native test discovery, workspace coverage and case-sensitive resources.
test-go: CASE_SENSITIVE_ENV := FLOWMARK_CASE_SENSITIVE_DIR

# The complete local install adds the locked board dependencies after the Kit bootstrap.
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

# Locked online dependency preparation, selected independently by each CI job.
.PHONY: flowmark-go-install flowmark-node-install flowmark-php-install
flowmark-go-install: ## Download the locked Go workspace dependencies; ONLINE
	cd packages/go && $(ONLINE) $(GO) mod download

flowmark-node-install: ## Install the board example npm lock; ONLINE
	cd examples/board/web && $(ONLINE) $(NPM) ci

flowmark-php-install: ## Install the board example Composer lock; ONLINE
	$(ONLINE) $(COMPOSER) install --no-interaction --working-dir=examples/board/web

# A parent benchmark runs through all of its sub-benchmarks: BenchmarkShardOpen took 31.9 s on an Apple M3 Pro, past the
# runner's 30 s default, so each test is given 120 s, about four times the slowest measured one.
.PHONY: flowmark-benchmarks
flowmark-benchmarks: ## Run writepath tests and benchmarks through the Kit runner; offline
	$(NODE) $(KIT)/run-tests.$(KIT_EXT) go --timeout 120 --cwd packages/go -- -bench . -benchmem ./writepath
