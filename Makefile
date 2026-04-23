BINARY := gen
BINDIR := bin
SRCDIR := ./cmd/gen
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
GOFILES := $(shell find . -path './vendor' -prune -o -path './.git' -prune -o -name '*.go' -print)
GOIMPORTS_VERSION := v0.43.0

.PHONY: build build-all install clean release test format format-check lint install-format-tools check-format-tools

build: format
	@mkdir -p $(BINDIR)
	go build $(LDFLAGS) -o $(BINDIR)/$(BINARY) $(SRCDIR)

build-all: format
	go build ./...

install: build
	@mkdir -p $(HOME)/.local/bin
	cp $(BINDIR)/$(BINARY) $(HOME)/.local/bin/

install-format-tools:
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

check-format-tools:
	@command -v goimports >/dev/null || { \
		echo "goimports is required. Install it with: make install-format-tools"; \
		exit 1; \
	}

format: check-format-tools
	@gofmt -w $(GOFILES)
	@goimports -w $(GOFILES)

format-check: check-format-tools
	@files="$$(gofmt -l $(GOFILES))"; \
	if [ -n "$$files" ]; then \
		echo "Go files are not formatted. Run: make format"; \
		echo "$$files"; \
		exit 1; \
	fi
	@files="$$(goimports -l $(GOFILES))"; \
	if [ -n "$$files" ]; then \
		echo "Go imports are not formatted. Run: make format"; \
		echo "$$files"; \
		exit 1; \
	fi

lint:
	go vet ./...
	@$(MAKE) format-check

test:
	go test ./...

clean:
	rm -rf $(BINDIR)

release: format
	@mkdir -p $(BINDIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_darwin_amd64 $(SRCDIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_darwin_arm64 $(SRCDIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_linux_amd64 $(SRCDIR)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINDIR)/$(BINARY)_linux_arm64 $(SRCDIR)
	cd $(BINDIR) && cp $(BINARY)_darwin_amd64 $(BINARY) && tar -czf $(BINARY)_darwin_amd64.tar.gz $(BINARY) && rm $(BINARY)
	cd $(BINDIR) && cp $(BINARY)_darwin_arm64 $(BINARY) && tar -czf $(BINARY)_darwin_arm64.tar.gz $(BINARY) && rm $(BINARY)
	cd $(BINDIR) && cp $(BINARY)_linux_amd64 $(BINARY) && tar -czf $(BINARY)_linux_amd64.tar.gz $(BINARY) && rm $(BINARY)
	cd $(BINDIR) && cp $(BINARY)_linux_arm64 $(BINARY) && tar -czf $(BINARY)_linux_arm64.tar.gz $(BINARY) && rm $(BINARY)
