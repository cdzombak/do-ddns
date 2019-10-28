SHELL:=/usr/bin/env bash

default: help

# via https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: check-env
check-env:
ifndef VERSION
	$(error env variable VERSION is missing)
endif

.PHONY: client-all
client-all: ## Build client binaries for supported platforms.
	mkdir -p out/linux_amd64
	env GOOS=linux GOARCH=amd64 go build -o out/linux_amd64/do-ddns-client ./client
	mkdir -p out/darwin_amd64
	env GOOS=darwin GOARCH=amd64 go build -o out/darwin_amd64/do-ddns-client ./client

.PHONY: server-all
server-all: ## Build server binaries for supported platforms.
	mkdir -p out/linux_amd64
	env GOOS=linux GOARCH=amd64 go build -o out/linux_amd64/do-ddns-server ./server
	mkdir -p out/darwin_amd64
	env GOOS=darwin GOARCH=amd64 go build -o out/darwin_amd64/do-ddns-server ./server

.PHONY: package
package: check-env client-all server-all  ## Build and package client & server binaries for supported platforms. Requires environment variable VERSION to be set.
	mkdir -p out/package
	tar -czvf out/package/do-ddns-$$VERSION-linux_amd64.tar.gz -C out/linux_amd64 .
	tar -czvf out/package/do-ddns-$$VERSION-darwin_amd64.tar.gz -C out/darwin_amd64 .

.PHONY: clean
clean: ## Remove build/package products.
	rm -rf out
