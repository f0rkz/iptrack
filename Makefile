.PHONY: build test run provider clean

GO_ENV = GOCACHE=$(CURDIR)/.cache/go-build GOPATH=$(CURDIR)/.cache/go GOFLAGS=-buildvcs=false

build:
	mkdir -p bin
	$(GO_ENV) go build -o bin/iptrack ./cmd/iptrack

test:
	$(GO_ENV) go test -race ./...

run:
	$(GO_ENV) go run ./cmd/iptrack

provider:
	mkdir -p bin
	cd terraform-provider-iptrack && GOCACHE=$(CURDIR)/.cache/provider-build GOPATH=$(CURDIR)/.cache/provider-go GOFLAGS=-buildvcs=false go build -o ../bin/terraform-provider-iptrack

clean:
	rm -rf bin .cache
