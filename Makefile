BINARY := gen
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build install clean release

build:
	go build $(LDFLAGS) -o $(BINARY) .

install: build
	sudo mv $(BINARY) /usr/local/bin/

clean:
	rm -f $(BINARY) $(BINARY)_*

release:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_darwin_amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)_darwin_arm64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)_linux_amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)_linux_arm64 .
	tar -czf $(BINARY)_darwin_amd64.tar.gz $(BINARY)_darwin_amd64
	tar -czf $(BINARY)_darwin_arm64.tar.gz $(BINARY)_darwin_arm64
	tar -czf $(BINARY)_linux_amd64.tar.gz $(BINARY)_linux_amd64
	tar -czf $(BINARY)_linux_arm64.tar.gz $(BINARY)_linux_arm64
