# MascotApp — task runner.
#
# Deletion policy: agents are denied direct `rm` by .claude/settings.json.
# Every legitimate deletion must live here, inside a reviewed target, so that
# what gets removed is a versioned artifact rather than a model's judgment call.
# Keep every path in `clean` explicit. Never introduce a variable path here.

.PHONY: help clean clean-api clean-web dev generate engram-index migrate migrate-down db-reset test test-api test-api-container test-short test-web test-race lint lint-api lint-web

help:
	@echo "clean       Remove build artifacts (explicit paths only)"
	@echo "generate    Regenerate Go + TS types from api/openapi.yaml"
	@echo "engram-index Regenerate docs/vault/20-arquitectura/indice-engram.md from .engram/queue"
	@echo "dev         Start the local stack"
	@echo "migrate     Apply pending migrations (needs DATABASE_URL)"
	@echo "migrate-down Roll back one migration (needs DATABASE_URL)"
	@echo "db-reset    Roll all the way down and back up (needs DATABASE_URL)"
	@echo "test        Run the full test suite (Go + web)"
	@echo "test-short  Go tests without the container-backed suite (fast, proves no isolation)"
	@echo "lint        Run all linters (Go + web)"
	@echo "test-race   Race detector (needs cgo + a C compiler; CI/Linux only)"

clean: clean-api clean-web

clean-api:
	rm -rf apps/api/bin
	rm -rf apps/api/coverage.out
	rm -rf apps/api/.gotmp

clean-web:
	rm -rf apps/web/dist
	rm -rf apps/web/node_modules/.vite
	rm -rf apps/web/coverage

# Regenerates every generated artifact from its single source: the HTTP types
# from api/openapi.yaml, and the query code from internal/db/query plus the
# migrations sqlc type-checks it against.
# The generated files are committed; CI re-runs this and fails on any diff,
# which is what stops the contract and the code from drifting apart.
generate:
	cd apps/api && oapi-codegen --config internal/api/config.yaml ../../api/openapi.yaml
	cd apps/api && sqlc generate
	cd apps/web && npm run generate

# Projects the Engram memory layer into the vault. Engram is reachable only
# through Claude Code's MCP server; the repository has to stand on its own for
# any other agent, so the index is generated rather than hand-maintained.
#
# It is a Go program and not a script in some other language because
# TestDevcontainerInstallsEveryToolTheMakefileInvokes checks that every tool a
# recipe runs is installed by .devcontainer/postCreate.sh, and it derives that
# list from `go install` lines only. A `python` recipe passed on the authoring
# host and failed the guard: correct, because it would fail on a fresh
# container. Do not "simplify" this back to an interpreter.
engram-index:
	cd apps/api && go run ../../scripts/engram-index.go

dev:
	docker compose up -d
	@echo "Local stack up. API: http://localhost:8080  Web: http://localhost:5173"

# --- Local database ---------------------------------------------------------
#
# DATABASE_URL is REQUIRED and deliberately has no default. A migration target
# that works without anyone choosing a database is one shell export away from
# migrating the wrong one, and `?=` would make that the quiet path. Export it
# from your .env, which is not committed:
#
#   export $(grep -v '^#' .env | xargs)   # or: set -a; . ./.env; set +a
#
# These targets run the goose CLI against internal/db/migrations -- the same
# directory //go:embed ships and sqlc reads. A test asserts the on-disk set and
# the embedded set are identical, so the CLI and the application cannot end up
# applying different SQL.
GOOSE_DIR := apps/api/internal/db/migrations

define require_database_url
	@if [ -z "$(DATABASE_URL)" ]; then 		echo "DATABASE_URL is not set. This target refuses to guess which database to"; 		echo "migrate. Export it from .env first -- see the Makefile comment above."; 		exit 1; 	fi
endef

migrate:
	$(require_database_url)
	goose -dir $(GOOSE_DIR) postgres "$(DATABASE_URL)" up

# One step, not all the way down. `goose down` is destructive against real data,
# so the blast radius stays as small as the command reads.
migrate-down:
	$(require_database_url)
	goose -dir $(GOOSE_DIR) postgres "$(DATABASE_URL)" down

# Owns local teardown, so nobody reaches for `rm` or `docker volume rm`. It
# tears down THROUGH the migration set -- reviewed SQL with a known blast radius
# -- rather than through a filesystem path that expands to whatever it expands
# to. The application roles and the citext extension survive by design (D1b).
db-reset:
	$(require_database_url)
	goose -dir $(GOOSE_DIR) postgres "$(DATABASE_URL)" down-to 0
	$(MAKE) migrate

# NOTE: `-race` requires cgo and a C compiler. Windows dev machines here have
# neither (CGO_ENABLED=0, no gcc), so local runs use plain `go test` and CI runs
# `test-race` on Linux, where the race detector works out of the box.
#
# GOTMPDIR: on this Windows host, Application Control blocks freshly linked test
# binaries under %LOCALAPPDATA%\Temp with "An Application Control policy has
# blocked this file". The build succeeds and the run never starts, so it reads
# like a test failure. Verified 2026-08-29: identical code passes when the link
# target moves into the repository. Harmless elsewhere -- GOTMPDIR just points
# Go's scratch space at a directory the policy allows.
#
# The scratch directory is also wiped before every run. UPDATE 2026-08-30: that
# was diagnosed as the fix and it is NOT. The block recurred against a freshly
# wiped .gotmp during T-01-009, T-01-011 and T-01-012, so neither relocating the
# directory nor wiping it is the cause:
#
#   fork/exec .../.gotmp/go-build.../b001/db.test.exe:
#       An Application Control policy has blocked this file.
#
# IDENTIFIED 2026-08-31 during T-01-020. It is WINDOWS SMART APP CONTROL, and it
# now blocks EVERY run rather than one in four:
#
#   Get-MpComputerStatus            -> AMRunningMode: Passive Mode
#                                      RealTimeProtectionEnabled: False
#   HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy
#     VerifiedAndReputablePolicyState -> 1   (0 off, 1 enforcement, 2 evaluation)
#
# Defender is not even active, so THE DEFENDER EXCLUSION THIS PROJECT KEPT
# RECOMMENDING WOULD HAVE DONE NOTHING. Smart App Control blocks unsigned
# executables with no reputation, and unlike Defender IT HAS NO EXCLUSION LIST
# AT ALL: it is on or off, and turning it off is irreversible without
# reinstalling Windows. Every `go test` links a fresh unsigned binary, so there
# is no in-repo workaround. Already-installed tools with established reputation
# -- golangci-lint, govulncheck, go vet -- keep working; only newly linked
# binaries are blocked, which is why linting never showed the symptom.
#
# Use `test-api-container` below. Three earlier diagnoses recorded here were
# wrong and are left in place on purpose: a confident wrong explanation is what
# cost three tasks' worth of workarounds.
#
# The wipe is kept because it costs nothing -- Go's real build cache is GOCACHE,
# which is untouched, so this forces no recompilation.
#
# Do NOT add `|| true` or a retry to these targets. If a run still fails with
# only a scratch-directory line and no `--- FAIL`, that is a new symptom worth
# reading, not worth suppressing.
GOTMPDIR_API := $(CURDIR)/apps/api/.gotmp

test: test-api test-web

# -p 1 runs one package at a time.
#
# It is not a speed knob, it is the fix for a real flake. Three packages start
# their own Postgres container, and when their test binaries open Docker
# Desktop's named pipe at the same moment, testcontainers' provider detection
# loses the race and reports `rootless Docker is not supported on Windows,
# failed to create Docker provider`. The whole package then fails at 0.00s,
# which reads like a broken migration and is not one.
#
# Measured on 2026-08-30 on the Windows host: 6 of 17 parallel runs hit it, 0 of
# 12 serialised runs did. This is the transient the comment in dbtest/container.go
# called unexplained on 2026-08-29; it now has a name.
#
# The cost is close to nothing -- the containers already dominate the wall clock
# and this project has five Go packages.
test-api:
	@rm -rf $(GOTMPDIR_API) && mkdir -p $(GOTMPDIR_API)
	cd apps/api && GOTMPDIR=$(GOTMPDIR_API) go test -p 1 ./...

# The same suite, run on Linux, where Smart App Control does not exist. On this
# Windows host it is currently the ONLY way to execute the tests at all.
#
# Two mounts are load-bearing and neither is obvious:
#
#   docker.sock  testcontainers starts Postgres as a SIBLING container on the
#                host daemon, not a child. TESTCONTAINERS_HOST_OVERRIDE is what
#                tells the suite to reach that sibling at host.docker.internal
#                instead of at 127.0.0.1, which inside this container is itself.
#
#   GOMODCACHE   the network here intercepts TLS, so module downloads fail with
#                `certificate signed by unknown authority`. Mounting the host's
#                already-populated module cache and setting GOPROXY=off skips
#                the download entirely. This is why -mod=mod is safe: nothing is
#                fetched, and go.mod is not rewritten.
#
# Ryuk is disabled because its reaper cannot see sibling containers reliably
# through a mounted socket; testcontainers still removes what it started.
#
# MSYS_NO_PATHCONV stops Git Bash rewriting the container-side paths into
# Windows ones, which fails with `working directory ... is invalid`.
test-api-container:
	MSYS_NO_PATHCONV=1 docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "$(CURDIR)":/src \
		-v "$$(go env GOMODCACHE)":/go/pkg/mod \
		-w /src/apps/api \
		-e GOFLAGS=-mod=mod -e GOPROXY=off \
		-e TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal \
		-e TESTCONTAINERS_RYUK_DISABLED=true \
		--add-host host.docker.internal:host-gateway \
		golang:1.27 go test -count=1 -p 1 ./...

# test-short skips every container-backed test. It is the fast inner loop and
# the path to use when the Docker daemon is down -- but it proves nothing about
# tenant isolation, so it is never the gate for calling a task done.
test-short:
	@rm -rf $(GOTMPDIR_API) && mkdir -p $(GOTMPDIR_API)
	cd apps/api && GOTMPDIR=$(GOTMPDIR_API) go test -short ./...

test-web:
	cd apps/web && npm run test -- --run

test-race:
	cd apps/api && CGO_ENABLED=1 go test -race ./...

lint: lint-api lint-web

lint-api:
	cd apps/api && golangci-lint run ./...
	cd apps/api && govulncheck ./...

lint-web:
	cd apps/web && npm run lint
	cd apps/web && npm run typecheck
