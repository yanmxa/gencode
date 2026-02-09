BINARY := gen
BINDIR := bin
SRCDIR := ./cmd/gen
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build install clean release

build:
	@mkdir -p $(BINDIR)
	go build $(LDFLAGS) -o $(BINDIR)/$(BINARY) $(SRCDIR)

install: build
	@mkdir -p $(HOME)/.local/bin
	cp $(BINDIR)/$(BINARY) $(HOME)/.local/bin/

clean:
	rm -rf $(BINDIR)

release:
	@mkdir -p $(BINDIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_darwin_amd64 $(SRCDIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_darwin_arm64 $(SRCDIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_linux_amd64 $(SRCDIR)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_linux_arm64 $(SRCDIR)
	cd $(BINDIR) && tar -czf $(BINARY)_darwin_amd64.tar.gz $(BINARY)_darwin_amd64
	cd $(BINDIR) && tar -czf $(BINARY)_darwin_arm64.tar.gz $(BINARY)_darwin_arm64
	cd $(BINDIR) && tar -czf $(BINARY)_linux_amd64.tar.gz $(BINARY)_linux_amd64
	cd $(BINDIR) && tar -czf $(BINARY)_linux_arm64.tar.gz $(BINARY)_linux_arm64
