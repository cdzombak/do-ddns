SHELL:=/usr/bin/env bash

default: package

# via https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: check-env
check-env:
ifndef VERSION
	$(error env variable VERSION is missing (set it to a string like "1.0.1"))
endif

.PHONY: build-client
build-client: check-env ## Build client binaries for supported platforms.
	mkdir -p out/linux_arm
	mkdir -p out/linux_mips64
	mkdir -p out/linux_amd64
	mkdir -p out/darwin_amd64
	env GOOS=linux GOARCH=amd64 go build -ldflags "-X main.BuildVersion=$$VERSION" -o out/linux_amd64/do-ddns-client ./client
	env GOOS=linux GOARCH=mips64 go build -ldflags "-X main.BuildVersion=$$VERSION" -o out/linux_mips64/do-ddns-client ./client
	env GOOS=linux GOARCH=arm go build -ldflags "-X main.BuildVersion=$$VERSION" -o out/linux_arm/do-ddns-client ./client
	env GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.BuildVersion=$$VERSION" -o out/darwin_amd64/do-ddns-client ./client

.PHONY: build-server
build-server: ## Build server binaries for supported platforms.
	mkdir -p out/linux_amd64
	env GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.BuildVersion=$$VERSION" -o out/darwin_amd64/do-ddns-server ./server
	env GOOS=linux GOARCH=amd64 go build -ldflags "-X main.BuildVersion=$$VERSION" -o out/linux_amd64/do-ddns-server ./server

.PHONY: build
build: build-client build-server  ## Build client & server binaries for supported platforms.

.PHONY: package
package: check-env clean build  ## Build & package client & server binaries for supported platforms. Requires environment variable VERSION to be set.
	mkdir -p out/package
	tar -czvf out/package/do-ddns-$$VERSION-linux_amd64.tar.gz -C out/linux_amd64 .
	tar -czvf out/package/do-ddns-$$VERSION-linux_mips64.tar.gz -C out/linux_mips64 .
	tar -czvf out/package/do-ddns-$$VERSION-linux_arm.tar.gz -C out/linux_arm .
	tar -czvf out/package/do-ddns-$$VERSION-darwin_amd64.tar.gz -C out/darwin_amd64 .

.PHONY: clean
clean: ## Remove build/package products.
	rm -rf out
