BINARY  := my-voice
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin
GOFLAGS ?=

.PHONY: build install uninstall clean vet

build:
	go build $(GOFLAGS) -o $(BINARY) .

install: build
	install -d $(BINDIR)
	install -m 755 $(BINARY) $(BINDIR)/$(BINARY)

uninstall:
	rm -f $(BINDIR)/$(BINARY)

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
