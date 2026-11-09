# The recipes of flowmark. Each recipe runs a utility of the installed package @polyspec/kit with its own arguments: KIT is
# the dist folder of that package, the same for a release install and for a link to the kit checkout, whose make kit-build
# writes it. kit includes no recipe here; the targets below are the ones that this repository, its CI and its documents call.
KIT := $(CURDIR)/node_modules/@polyspec/kit/dist
ONLINE ?=

# The suite of the repository: the checks and the tests that config/owner-checks.json selects from.
CHECK_TARGETS := layout-check docs-check rules-check owner-validate rebuild-check language-check duplication-check link-check suppression-check lint-ts lint-go refs-check internal-paths-check test-go test-infrastructure

# ---- Tools ------------------------------------------------------------------------------------------------------------
# The tools of mise.toml run from the shims of the mise that make install-tools TOOLS=npm installs into var/tools, the
# checkout's own (a linked worktree installs its own). The shims read the environment of that mise, so every recipe runs with
# it, and with var/tools/bin and the shims first on PATH. The settings of mise and uv are those of mise.toml.
KIT_TOOLS := $(CURDIR)/var/tools
export MISE_DATA_DIR := $(KIT_TOOLS)/mise
export MISE_CACHE_DIR := $(KIT_TOOLS)/mise/cache
export MISE_STATE_DIR := $(KIT_TOOLS)/mise/state
export MISE_CONFIG_DIR := $(KIT_TOOLS)/mise/config
export MISE_GLOBAL_CONFIG_FILE := $(CURDIR)/mise.toml
export MISE_GLOBAL_CONFIG_ROOT := $(CURDIR)
MISE_ENVIRONMENT := MISE_DATA_DIR MISE_CACHE_DIR MISE_STATE_DIR MISE_CONFIG_DIR MISE_GLOBAL_CONFIG_FILE MISE_GLOBAL_CONFIG_ROOT
# On Windows a shim of mise starts mise.exe by its PATH name, so the folder of the mise binary is on PATH too, and the recipes,
# which Git Bash runs, start node.exe and the npm-cli.js of the npm of mise.toml in place of their .cmd wrappers.
ifeq ($(OS),Windows_NT)
# kit 0.0.98's run-tests dropped NODE_TEST_CONTEXT, so a node --test that a recipe starts from inside a node test runs as
# its own run, not as a child of that test (kit K1094); no recipe passes it on.
unexport NODE_TEST_CONTEXT
export PATH := $(KIT_TOOLS)/bin:$(KIT_TOOLS)/mise-release/mise/bin:$(KIT_TOOLS)/mise/shims:$(PATH)
MISE_BINARY := $(KIT_TOOLS)/mise-release/mise/bin/mise.exe
ifneq ($(wildcard $(MISE_BINARY)),)
MISE_ENVIRONMENT_ARGS := $(foreach variable,$(MISE_ENVIRONMENT),$(variable)="$($(variable))")
NODE_DIRECTORY := $(shell $(MISE_ENVIRONMENT_ARGS) "$(MISE_BINARY)" where node)
NPM_DIRECTORY := $(shell $(MISE_ENVIRONMENT_ARGS) "$(MISE_BINARY)" where npm:npm)
endif
NODE := $(if $(NODE_DIRECTORY),$(subst \,/,$(NODE_DIRECTORY))/node.exe,$(KIT_TOOLS)/bin/node)
NPM := $(if $(NPM_DIRECTORY),$(NODE) $(subst \,/,$(NPM_DIRECTORY))/node_modules/npm/bin/npm-cli.js,$(KIT_TOOLS)/bin/npm)
else
export PATH := $(KIT_TOOLS)/bin:$(KIT_TOOLS)/mise/shims:$(PATH)
NODE := $(KIT_TOOLS)/bin/node
NPM := $(KIT_TOOLS)/bin/npm
endif
ALINT := $(KIT_TOOLS)/mise/shims/alint
GO := $(firstword $(wildcard $(KIT_TOOLS)/mise/shims/go.exe $(KIT_TOOLS)/mise/shims/go.cmd $(KIT_TOOLS)/mise/shims/go))

# ---- Install ----------------------------------------------------------------------------------------------------------
# The order of a checkout: the mise, node and npm of mise.toml (the bootstrap of kit, which the Node.js of the machine runs,
# since var/tools has no node yet), the packages of package-lock.json, the other tools of mise.toml, then the links of
# .links. Each step is a make of its own, so it reads the tools that the step before installed.
.PHONY: install install-tools install-packages install-link toolchain-lock
install: ## Install the tools, the packages and the links of the checkout, in that order; ONLINE
	$(MAKE) --no-print-directory install-tools TOOLS=npm
	$(MAKE) --no-print-directory install-packages
	$(MAKE) --no-print-directory install-tools
	$(MAKE) --no-print-directory install-link

install-tools: ## Install the locked tools of mise.toml, or those that TOOLS names; TOOLS=npm installs mise, node and npm; ONLINE
ifeq ($(TOOLS),npm)
	$(ONLINE) node $(KIT)/bootstrap/bootstrap.mjs
else
	$(ONLINE) $(NODE) $(KIT)/install-tools.mjs --no-self-update $(TOOLS)
endif

install-packages: ## Install the packages of package-lock.json, @polyspec/kit included, with the npm of var/tools; ONLINE
	$(ONLINE) $(NPM) ci

install-link: ## Link the installed packages to the checkouts that .links names; PROJECT and RUNTIME select as for make link
	$(NODE) $(KIT)/link.mjs link$(if $(PROJECT), --project $(PROJECT))$(if $(RUNTIME), --runtime $(RUNTIME))

# mise locks a pypi: tool with the first uv on PATH, so it locks and installs uv and python first, and locks the rest with
# that Python as UV_PYTHON; mise records the archive of each platform that has one.
TOOLCHAIN_LOCK_PLATFORMS := linux-x64,linux-arm64,macos-arm64,macos-x64,windows-x64,windows-arm64
toolchain-lock: ## Write mise.lock with mise lock for each platform of linux, macOS and Windows on x64 and arm64; ONLINE
	$(ONLINE) $(KIT_TOOLS)/bin/mise lock uv python --global --platform $(TOOLCHAIN_LOCK_PLATFORMS)
	$(ONLINE) $(KIT_TOOLS)/bin/mise install --locked uv python
	UV_PYTHON="$$($(KIT_TOOLS)/bin/mise which python3)" $(ONLINE) $(KIT_TOOLS)/bin/mise lock --global --platform $(TOOLCHAIN_LOCK_PLATFORMS)

# The locked online dependencies of each language, each the locked install of its package manager.
.PHONY: deps-go
deps-go: ## Download the locked Go workspace dependencies; ONLINE
	cd packages/go && $(ONLINE) $(GO) mod download

# ---- Links of 0.0.x development (.links) ------------------------------------------------------------------------------
.PHONY: link link-check unlink worktree-setup
link: ## Link the installed packages to the checkouts that .links names (PROJECT names the consumer); offline
	$(NODE) $(KIT)/link.mjs link$(if $(PROJECT), --project $(PROJECT))$(if $(RUNTIME), --runtime $(RUNTIME))

link-check: ## Fail while the links differ from .links; offline
	$(NODE) $(KIT)/link-check.mjs$(if $(PROJECT), --project $(PROJECT))

unlink: ## Remove the links of PROJECT and the go.work files that link wrote; make install restores the locked installs; offline
	$(NODE) $(KIT)/link.mjs unlink$(if $(PROJECT), --project $(PROJECT))$(if $(RUNTIME), --runtime $(RUNTIME))

# The worktree of a row (kit K1107), as kit 0.0.98 made it: ../<main checkout>-<NAME>-<ID> on <TYPE or fix>/<NAME>-<ID> from main;
# a worktree without node_modules first gets npm ci of the npm of var/tools, then make install and make link-check run in it. The
# worktree runs its own tools in a make of its own (kit K858): the recipe drops the flags of this make and this var/tools from PATH.
WORKTREE_MAIN = $(patsubst %/.git,%,$(shell git rev-parse --path-format=absolute --git-common-dir))
WORKTREE_DECLARATION = --folder $(WORKTREE_MAIN)-$(NAME)-$(ID) --branch $(or $(TYPE),fix)/$(NAME)-$(ID) --base main --prepare 'test -d node_modules || PATH="$(KIT_TOOLS)/bin:$$PATH" $(NPM) ci --ignore-scripts' --make install --make link-check
WORKTREE_ENVIRONMENT = env -u MAKEFLAGS -u MFLAGS -u MAKELEVEL PATH="$$(printf '%s' "$$PATH" | awk -v RS=: -v ORS=: -v tools='$(KIT_TOOLS)' 'index($$0, tools) != 1' | sed 's/:$$//')"
worktree-setup: ## Create or reuse the worktree NAME of the row ID (TYPE, fix by default) from main, then make install and make link-check in it; ONLINE
	$(ONLINE) $(WORKTREE_ENVIRONMENT) $(NODE) $(KIT)/worktree-setup.mjs $(WORKTREE_DECLARATION)

# ---- Build and test ---------------------------------------------------------------------------------------------------
.PHONY: build build-go flowmark-build test-go test-infrastructure flowmark-node-check flowmark-go-check flowmark-test flowmark-benchmarks
build: build-go flowmark-build ## Build the Go workspace and the executable that sidecar.json names; offline

build-go: ## Build every module of the tracked go.work; offline
	$(NODE) $(KIT)/runtime-targets.mjs build --roots packages/go tests/conformance/go -- $(GO) build ./...

flowmark-build: ## Build the executable that sidecar.json names into build/flowmark; offline
	cd packages/go && $(GO) build -o $(CURDIR)/build/flowmark ./cmd/flowmark

# kit's runner runs the tests of the named roots, and prepares and cleans up the case-sensitive folder that
# FLOWMARK_CASE_SENSITIVE_DIR names. The runner arguments are those that kit 0.0.98 passed (kit K1094, K1106): one go test with
# -count=1 -timeout=0, and node one test at a time with the 30 s test timeout.
test-go: ## Run the unit tests of every module of go.work, with the name filter CASES=<pattern>; offline
	CASE_SENSITIVE_ENV=FLOWMARK_CASE_SENSITIVE_DIR $(NODE) $(KIT)/runtime-targets.mjs test --runner go --roots packages/go tests/conformance/go -- -count=1 -timeout=0

test-infrastructure: ## Run every tracked root Node test through the runner of kit; offline
	CASE_SENSITIVE_ENV=FLOWMARK_CASE_SENSITIVE_DIR $(NODE) $(KIT)/runtime-targets.mjs infrastructure tests -- --test-timeout=30000 --test-concurrency=1

flowmark-node-check: test-infrastructure ## Run the Node suite of the root infrastructure; offline
flowmark-go-check: build-go test-go ## Build and test the complete Go workspace; offline
flowmark-test: flowmark-node-check flowmark-go-check ## Run the Node and the Go suites; offline

# A parent benchmark runs through all of its sub-benchmarks: BenchmarkShardOpen took 31.9 s on an Apple M3 Pro, past the
# runner's 30 s default, so each test is given 120 s, about four times the slowest measured one.
flowmark-benchmarks: ## Run the writepath tests and benchmarks and the statement benchmark through the runner of kit; offline
	$(NODE) $(KIT)/run-tests.mjs go --timeout 120 --cwd packages/go -- -count=1 -timeout=0 -bench . -benchmem ./writepath
	$(NODE) $(KIT)/run-tests.mjs go --timeout 600 --cwd packages/go -- -count=1 -timeout=0 -run '^$$' -bench BenchmarkStatement -benchtime 20x ./statement

.PHONY: slug-vectors
slug-vectors: ## Record the heading ids GitHub, VS Code and GitLab render into store/testdata/slug; needs the network, gh, npm and cargo
	$(NODE) scripts/record-slugs.mts

# ---- Lint: each tool reads the configuration of this repository by its own discovery: .oxlintrc.json and knip.json, and
# .golangci.yml -------------------------------------------------------------------------------------------------
.PHONY: lint-ts lint-go
lint-ts: ## Lint the TypeScript: oxlint and knip from the root; offline
	$(NODE) $(KIT)/lint-ts.mjs

lint-go: ## Lint each Go module: golangci-lint and deadcode -test; offline once the modules are downloaded
	$(NODE) $(KIT)/lint-go.mjs

# ---- Checks -----------------------------------------------------------------------------------------------------------
.PHONY: layout-check docs-check rules-check owner-check owner-validate rebuild-check language-check duplication-check suppression-check refs-check internal-paths-check commit-msg-check

layout-check: ## Check the folder layout against config/layout.yml with alint, that Git ignores each install folder and that no tracked file is generated; offline
	$(ALINT) check --config config/layout.yml --color never --compact .
	$(NODE) $(KIT)/generated-files.mjs installs var package.json=node_modules pyproject.toml=.venv composer.json=vendor
	$(NODE) $(KIT)/generated-files.mjs tracked --binary ELF --binary Mach-O --binary PE

docs-check: ## Check the documents of config/documents.json and the checklists of config/checklist.json; offline
	$(NODE) $(KIT)/check-documents.mjs --checklist-place --private-path /Users/ --private-path /home/ --kit-rows

rules-check: ## Check that every rule of PRINCIPLES.md and AGENTS.md names the check that enforces it; offline
	$(NODE) $(KIT)/check-rules.mjs

owner-check: ## Run the checks that own the changed paths (config/owner-checks.json); PATHS="a b" or BASE=<revision> selects the paths
	$(NODE) $(KIT)/owner-check.mjs --every-path $(if $(PATHS),--paths "$(PATHS)") $(if $(BASE),--base $(BASE))

owner-validate: ## Check that config/owner-checks.json owns every tracked path and names only existing targets; offline
	$(NODE) $(KIT)/owner-check.mjs --every-path --validate

# The tool sources of rebuild-check, the values that kit 0.0.98 fixed (kit K1080): the scripts and generators, each glob quoted.
REBUILD_SOURCES := --source 'scripts/**/*.{mts,mjs,cjs,js,ts,py,php,sh,mk}' --source 'packages/*/{scripts,tools,bin}/**/*.{mts,mjs,cjs,js,ts,py,php,sh,mk}' --source 'packages/*/*/{scripts,tools,bin}/**/*.{mts,mjs,cjs,js,ts,py,php,sh,mk}' --source '**/*_generate.go' --source '**/*.generate.{mts,mjs,cjs,js,ts,py,php,sh,mk}' --source 'benchmarks/**/*.{mts,mjs,cjs,js,ts,py,php,sh,mk}' --source 'examples/**/*.{mts,mjs,cjs,js,ts,py,php,sh,mk}' --source 'tests/fixtures/*/generate.*'
# The files among the sources that are no tool source (kit K1080): the .d.ts declarations and the tests.
REBUILD_EXCLUDES := --exclude '**/*.d.ts' --exclude '*/**/{test,tests}/**' --exclude 'test/**' --exclude 'tests/**/{*_generate.go,*.generate.*}' --exclude '**/*.{test,spec}.{js,jsx,ts,tsx,cjs,cts,mjs,mts}' --exclude '**/{*_test.go,test_*.py,*_test.py,*Test.php}'
rebuild-check: ## Check that every tool that config/rebuild.json selects has its class (S1); offline
	$(NODE) $(KIT)/rebuild-check.mjs $(REBUILD_SOURCES) $(REBUILD_EXCLUDES)

language-check: ## Fail on Hangul in a comment, an identifier, a message or a test name of the code; offline
	$(NODE) $(KIT)/language-check.mjs --forbid '\p{Script=Hangul}'

duplication-check: ## Fail while a piece of 6 lines or more of code exists twice outside the folders of config/duplication.json; offline
	$(NODE) $(KIT)/duplication-check.mjs

suppression-check: ## Fail on a tracked suppression of a lint finding that states no reason; offline
	$(NODE) $(KIT)/suppression-check.mjs

refs-check: ## Check that every task ID a document names resolves to a checklist item; offline
	$(NODE) scripts/check-refs.mts

internal-paths-check: ## Check that no tracked file names a dependency's source or build output by its path (K1002); offline
	$(NODE) scripts/check-internal-paths.mts

commit-msg-check: ## Check the commit message MESSAGE and the index against the commit rules of kit; offline
	$(NODE) $(KIT)/check-commits.mjs --message "$(MESSAGE)" --rules commit-ai-footer,commit-ai-identity

# ---- CI: the gates job of .github/workflows/ci.yml installs every tool and locked dependency that the gates need, then runs
# them past failures through ci-targets, which writes the report CI_REPORT ------------------------------------------------
CI_REPORT ?= var/report/ci-targets
GATES := layout-check docs-check rules-check owner-validate rebuild-check language-check duplication-check suppression-check lint-go lint-ts
# The checks that read the documents that ci.yml ignores (T90): the documents workflow (documents.yml) runs them on every push
# and pull request, and the gates run them too, so the CI run that a release takes holds them.
DOCUMENT_CHECKS := layout-check docs-check rules-check owner-validate refs-check internal-paths-check checklist-check
.PHONY: ci-targets gates gates-setup documents documents-setup diff-size-check checklist-check checklist-render
ci-targets: ## Run TARGETS past failures and write CI_REPORT (logs, record.json, summary.md)
	$(NODE) $(KIT)/ci-targets.mjs -k $(CI_REPORT) $(TARGETS)

gates: ## Run the gates past failures through ci-targets and write CI_REPORT
	$(NODE) $(KIT)/ci-targets.mjs -k $(CI_REPORT) $(GATES)

gates-setup: ## Install the tools of mise.toml and the locked Go dependencies, and build the Go workspace; ONLINE
	$(MAKE) --no-print-directory install-tools
	$(MAKE) --no-print-directory deps-go build-go

documents: ## Run the document checks past failures through ci-targets and write CI_REPORT
	$(NODE) $(KIT)/ci-targets.mjs -k $(CI_REPORT) $(DOCUMENT_CHECKS)

documents-setup: ## Install alint and go of mise.toml, the tools of the document checks besides node, and the locked Go dependencies; ONLINE
	$(MAKE) --no-print-directory install-tools TOOLS="alint go"
	$(MAKE) --no-print-directory deps-go

# The checklist is the corpus docs/plans/flowmark/checklist/, and checklist.md and its twin are its views (T61).
checklist-check: flowmark-build ## Check the catalog of the checklist corpus by its cases, and fail when a view differs from what the corpus renders; offline
	cd packages/go && $(GO) test ./ops -run '^TestChecklist' -count=1
	FLOWMARK=$(CURDIR)/build/flowmark $(NODE) scripts/checklist.mts check flowmark

checklist-render: flowmark-build ## Write checklist.md and checklist.ko.md from the checklist corpus; offline
	FLOWMARK=$(CURDIR)/build/flowmark $(NODE) scripts/checklist.mts render flowmark

diff-size-check: ## Fail while the diff of the GitHub event GITHUB_EVENT_NAME at GITHUB_EVENT_PATH holds more than the 3000 files of GitHub's path filter; offline
	$(NODE) scripts/check-diff-size.mts

# ---- Plans and commits ------------------------------------------------------------------------------------------------
# The plan utility reads its values from the environment, where make puts its command-line variables. The texts of a row reach
# it unexpanded: each text that is set is passed as its raw value, in single quotes, and an unset text stays unset.
raw_text = $(foreach v,$(1),$(if $(filter undefined,$(origin $(v))),,$(v)='$(subst ','\'',$(value $(v)))'))
.PHONY: plan-new plan-split item-new item-edit item-state item-test commit documents-stamp text-absent twin-merge land
plan-new: ## Write the plan NAME=<slug> with TITLE and TITLE_KO into docs/plans/<slug>/; offline
	NAME_PATTERN='^[a-z][a-z0-9-]*$$' $(NODE) $(KIT)/plan.mjs plan-new

plan-split: ## Move the items IDS from the checklist of the plan FROM to the plan TO, in both languages; offline
	$(NODE) $(KIT)/plan.mjs plan-split

item-new: ## Add the item ID to the checklist of PLAN and its Korean twin with TASK, TASK_KO, DELIVERABLES and VERIFICATION; offline
	$(call raw_text,TASK TASK_KO DELIVERABLES VERIFICATION) $(NODE) $(KIT)/plan.mjs item-new

item-edit: ## Replace the cells of the item ID with TASK, TASK_KO, DELIVERABLES, VERIFICATION, CAUSE, RETRY, CAUSE_KO or RETRY_KO; offline
	$(call raw_text,TASK TASK_KO DELIVERABLES VERIFICATION CAUSE RETRY CAUSE_KO RETRY_KO) DONE_STAYS=1 $(NODE) $(KIT)/plan.mjs item-edit

item-state: ## Move the item ID to STATE in both languages; offline
	$(NODE) $(KIT)/plan.mjs item-state

item-test: ## Run the commands of the Verification cell of the item ID in order; offline
	$(NODE) $(KIT)/plan.mjs item-test

commit: ## Commit the index for the item ID as TYPE(SCOPE): SUBJECT (#ID) with the body of BODY_FILE; offline
	$(NODE) $(KIT)/plan.mjs commit

documents-stamp: ## Write the source-sha256 of each English document into the Korean twins that PATHS names; offline
	$(NODE) $(KIT)/documents-stamp.mjs $(PATHS)

# FILE and TEXT reach the tool through its environment, unexpanded, since a stale text may hold a backquote, a $ or a quote.
unexport TEXT
text-absent: export KIT_TEXT_ABSENT_FILE = $(value FILE)
text-absent: export KIT_TEXT_ABSENT_TEXT = $(value TEXT)
text-absent: ## Fail while the tracked file FILE holds the stale text TEXT, naming each line; offline
	$(NODE) $(KIT)/text-absent.mjs

# The merge drivers kit-twin, kit-changelog and kit-checklist of the Git configuration run it with MERGE="%O %A %B %P".
twin-merge: ## Merge a Korean twin, a changelog or a checklist for Git with MERGE="<base> <ours> <theirs> <path>"; offline
	@$(NODE) $(KIT)/twin-merge.mjs $(MERGE)

land: ## Land the branch BRANCH of its worktree on main by a fast-forward merge, then remove the worktree and the branch; offline
	$(NODE) $(KIT)/land.mjs $(BRANCH) main

# ---- Release (.github/workflows/release.yml, config/release.json) -----------------------------------------------------
# The release uses the released versions, so its Go runs without the go.work of the links; the archive build builds the tree of
# the repository, whose tracked go.work is its own workspace.
.PHONY: release-tag release-tools release-tools-path release-assets release-publish release-undraft
release-tag release-tools release-publish release-undraft: export GOWORK := off
release-assets: export GOWORK = $(CURDIR)/go.work

# The run check (scripts/check-release-run.mts, T90) comes first: kit's release-run takes a successful run of any workflow, and
# a release takes a run of ci.yml alone. With AUTO the workflow checked out the head commit of the run, which the check compares.
release-tag: ## Tag vX.Y.Z of the change log and packages/go/vX.Y.Z at the commit of the successful ci.yml run RUN of main; AUTO releases only a version commit
	@test -n "$$RUN" || { echo "release-tag: RUN is required, for example make release-tag RUN=123"; exit 1; }
	$(NODE) scripts/check-release-run.mts "$$RUN"$(if $(AUTO), --commit "$$(git rev-parse HEAD)")
	RELEASE_MAIN=origin/main $(NODE) $(KIT)/release-run.mjs tag "$$RUN"$(if $(AUTO), --auto)

release-tools: ## Install the tools that config/release.json needs for the release; ONLINE
	$(ONLINE) $(NODE) $(KIT)/release-tools.mjs

release-tools-path: ## Append var/tools/bin, the shims of mise and the environment of mise to GITHUB_PATH and GITHUB_ENV for the steps that follow
	@test -n "$$GITHUB_PATH" && test -n "$$GITHUB_ENV" || { echo 'release-tools-path: run it in the release workflow, which sets GITHUB_PATH and GITHUB_ENV'; exit 1; }
	@echo "$(KIT_TOOLS)/mise/shims" >> "$$GITHUB_PATH"
	@echo "$(KIT_TOOLS)/bin" >> "$$GITHUB_PATH"
	@for name in $(MISE_ENVIRONMENT); do echo "$$name=$$(printenv $$name)"; done >> "$$GITHUB_ENV"

release-assets: ## Build the archives of TAG into var/release/assets; ONLINE
	@test -n "$$TAG" || { echo "release-assets: TAG is required, for example make release-assets TAG=v0.0.1"; exit 1; }
	RELEASE_ASSETS=var/release/assets $(ONLINE) $(NODE) $(KIT)/release.mjs assets "$$TAG"

release-publish: ## Create the GitHub Release of TAG as a draft with its notes and the files of release-assets
	@test -n "$$TAG" || { echo "release-publish: TAG is required, for example make release-publish TAG=v0.0.1"; exit 1; }
	RELEASE_ASSETS=var/release/assets RELEASE_RECORDS=var/release $(NODE) $(KIT)/release.mjs publish "$$TAG"

release-undraft: ## Make the draft release of TAG public
	@test -n "$$TAG" || { echo "release-undraft: TAG is required, for example make release-undraft TAG=v0.0.1"; exit 1; }
	RELEASE_RECORDS=var/release $(NODE) $(KIT)/release.mjs undraft "$$TAG"
