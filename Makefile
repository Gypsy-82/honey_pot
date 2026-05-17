BINARY   := trackerd
GOFLAGS  := -ldflags="-s -w" -trimpath
GOBIN    := $(HOME)/go/bin/go
GOPATH   := $(HOME)/gopath
GOROOT   := $(HOME)/go

.PHONY: build build-linux build-arm build-mac clean

build:
	GOPATH=$(GOPATH) GOROOT=$(GOROOT) $(GOBIN) build $(GOFLAGS) -o $(BINARY) .

build-linux:
	GOPATH=$(GOPATH) GOROOT=$(GOROOT) GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		$(GOBIN) build $(GOFLAGS) -o $(BINARY)_linux_amd64 .

build-arm:
	GOPATH=$(GOPATH) GOROOT=$(GOROOT) GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		$(GOBIN) build $(GOFLAGS) -o $(BINARY)_linux_arm64 .

build-mac:
	GOPATH=$(GOPATH) GOROOT=$(GOROOT) GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
		$(GOBIN) build $(GOFLAGS) -o $(BINARY)_darwin_arm64 .

clean:
	rm -f $(BINARY) $(BINARY)_linux_amd64 $(BINARY)_linux_arm64 $(BINARY)_darwin_arm64
