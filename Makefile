.PHONY: all install clean

# Just run: make
all:
	@cd web && bun install --silent && bun run build
	@go build -o bin/glog ./cmd/glog
	@./bin/glog serve --web web/build

install:
	@go build -o bin/glog ./cmd/glog
	@ln -sf $(PWD)/bin/glog $(shell go env GOPATH)/bin/glog

clean:
	@rm -rf bin/ web/build/ web/.svelte-kit/
