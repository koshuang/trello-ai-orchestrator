.PHONY: all build test test-scenario test-http test-unit clean run

GO      := go
BINARY  := trello-orchestrator
MODULE  := github.com/koshuang/trello-ai-orchestrator

# Suppress interactive prompts
export CI             := true
export GIT_PAGER      := cat
export PAGER          := cat

all: build test

# --------------- Build ---------------
build:
	$(GO) build -o $(BINARY) .

# --------------- Test suites ---------------
test: test-unit test-scenario test-http

# Scenario tests (15 scenarios covering @mention, fixed commands, safe mode, etc.)
test-scenario:
	$(GO) test -v -run "Scenario" ./usecase/ -count=1

# HTTP-level integration tests (real Gin server + HTTP requests)
test-http:
	$(GO) test -v -run "Integration" ./adapter/controller/ -count=1

# Legacy unit tests
test-unit:
	$(GO) test -v -run "Test" ./usecase/ -count=1

# Run all tests with short output (pass/fail only)
test-short:
	$(GO) test ./... -count=1

# --------------- Run ---------------
run: build
	TRELLO_API_KEY=placeholder \
	TRELLO_TOKEN=placeholder \
	PORT=8082 \
	./$(BINARY)

# --------------- Clean ---------------
clean:
	rm -f $(BINARY)
	rm -f orchestrator.db
	rm -rf usecase/docs/

# --------------- Diagnostics ---------------
vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy
