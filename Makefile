BINARY  := gopher
CMD     := ./cmd/gopher
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.Version=$(VERSION)

# benchmark selector and extra flags, ex. make bench BENCH=Generate BENCHFLAGS='-count 10'
BENCH      ?= .
BENCHFLAGS ?=

.PHONY: build install run test test-short bench bench-quick golden vet fmt tidy docs clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

install:
	go install -ldflags "$(LDFLAGS)" $(CMD)

run:
	go run $(CMD)

test:
	go test ./...

test-short:
	go test -short ./...

# -run '^$$' matters: without it the whole test suite, compile checks included,
# runs before the first benchmark
bench:
	go test -run '^$$' -bench '$(BENCH)' -benchmem $(BENCHFLAGS) ./...

# proves every benchmark compiles and runs, the timings are meaningless
bench-quick:
	go test -run '^$$' -bench '$(BENCH)' -benchmem -benchtime 10x ./...

golden:
	GOPHER_UPDATE_GOLDEN=1 go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

# paths the docs name on purpose that do not exist: files the adding-a-type
# walkthrough tells the reader to create
DOC_PLACEHOLDERS := templates/files/adapter/redis.tmpl templates/files/middleware/handler.tmpl

# check the docs for paths, links, and symbols that no longer exist. Only
# repo-rooted paths are checked; bare filenames in prose are not paths
docs:
	@fail=0; \
	for p in $$(grep -rhoE '`(cmd|internal|templates|docs)/[a-zA-Z0-9_./*-]+`' docs README.md CLAUDE.md | tr -d '`' | sort -u); do \
		case " $(DOC_PLACEHOLDERS) " in *" $$p "*) continue;; esac; \
		[ -e "$$p" ] || { echo "MISSING PATH: $$p"; fail=1; }; \
	done; \
	for l in $$(grep -rhoE '\]\(([^)#][^)]*)\)' docs | sed 's/^](//;s/)$$//' | grep -v '^http' | sort -u); do \
		[ -e "docs/$$l" ] || [ -e "$$l" ] || { echo "BROKEN LINK: $$l"; fail=1; }; \
	done; \
	for s in ModeCreate ModeAppend ModeEnsure StatusUnchanged StatusAppended TemplateData \
	         GenSpec TemplateRef RequiresModule splitPositional AdapterKinds ValobjKinds \
	         ModuleKinds UpdateEnv ErrFileExists ErrNilDependency FindModule \
	         BenchmarkGenerate BenchmarkGenerateCold BenchmarkGenerateWrite \
	         BenchmarkGenerateOverrides \
	         BenchmarkStartup BenchmarkRun XdgConfigEnv \
	         TestBenchRequestsProduceOutput benchRequests; do \
		grep -rq "$$s" cmd internal || { echo "STALE SYMBOL: $$s"; fail=1; }; \
	done; \
	[ $$fail -eq 0 ] && echo "docs ok"

clean:
	rm -rf bin
