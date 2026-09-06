SHELL := /bin/bash

PORT ?= 8080
SMOKE_PORT ?= 18080
SESSION ?= smoke
PRESET ?= dashboard
SOCK ?=

PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: help daemon client smoke-http smoke-ipc test test-race test-reattach test-startup-stress build demo install uninstall

help:
	@printf '%s\n' \
	  'A2UI developer commands:' \
	  '  make build               Compile a2ui and a2uid binaries to bin/.' \
	  '  make install             Install binaries to $(BINDIR).' \
	  '  make uninstall           Remove installed binaries from $(BINDIR).' \
	  '  make demo                Run the automated Agent round-trip demo.' \
	  '  make daemon              Start a foreground daemon and print its socket path.' \
	  '  make client SOCK=/path   Attach the Bubble Tea client to an existing socket.' \
	  '  make smoke-http          Verify MCP hello and operation over HTTP.' \
	  '  make smoke-ipc           Verify Unix-socket hello and snapshot delivery.' \
	  '  make test                Run the complete Go test suite.' \
	  '  make test-race           Run the complete race test suite.' \
	  '  make test-reattach       Run the persistent daemon reattach E2E test.' \
	  '  make test-startup-stress Run 500 concurrent Unix listener iterations.'

build:
	@mkdir -p bin
	go build -o bin/a2ui ./cmd/a2ui
	go build -o bin/a2uid ./cmd/a2uid

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 bin/a2ui $(DESTDIR)$(BINDIR)/a2ui
	install -m 0755 bin/a2uid $(DESTDIR)$(BINDIR)/a2uid

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/a2ui
	rm -f $(DESTDIR)$(BINDIR)/a2uid

demo:
	./scripts/demo/roundtrip.sh

daemon client:
	@PORT='$(PORT)' SESSION='$(SESSION)' PRESET='$(PRESET)' SOCK='$(SOCK)' ./scripts/dev/a2ui $@

smoke-http smoke-ipc:
	@PORT='$(SMOKE_PORT)' SESSION='$(SESSION)' PRESET='$(PRESET)' ./scripts/dev/a2ui $@

test:
	go test ./... -count=1

test-race:
	go test -race ./... -count=1

test-reattach:
	go test ./e2e -run '^TestPersistentDaemonReattachPreservesSemanticStateAndRemotePublicationBarrier$$' -count=1

test-startup-stress:
	go test ./ipc -run '^TestConcurrentListenUnixNeverUnlinksWinningLiveDaemon$$' -count=500
