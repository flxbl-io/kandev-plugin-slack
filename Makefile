.PHONY: build run test fmt vet package package-host clean

BIN := bin/kandev-plugin-slack
VERSION := 0.2.2
STAGE := .build/stage
PKG_OUT := kandev-plugin-slack-$(VERSION).tar.gz

## Build the plugin binary for the host platform. Development use only —
## the installed-plugin path always goes through package/package-host.
build:
	mkdir -p bin
	go build -o $(BIN) ./server

## Build + run. Mainly a smoke check: kandev normally spawns this binary
## itself over the go-plugin handshake, so a bare run just blocks.
run: build
	./$(BIN)

test:
	go test ./server

fmt:
	gofmt -l .

vet:
	go vet ./server

## Cross-compile every platform declared in manifest.yaml's
## runtime.executables, stage manifest.yaml + ui/ alongside them, and pack the
## tree with github.com/kandev/kandev/cmd/plugin-pack (resolved through this
## repo's local `replace` directive — see go.mod).
package:
	rm -rf $(STAGE)
	mkdir -p $(STAGE)/server $(STAGE)/ui
	cp manifest.yaml $(STAGE)/manifest.yaml
	cp README.md $(STAGE)/README.md
	cp slack-app-manifest.yaml $(STAGE)/slack-app-manifest.yaml
	cp ui/bundle.js $(STAGE)/ui/bundle.js
	GOOS=linux   GOARCH=amd64 go build -o $(STAGE)/server/plugin-linux-amd64       ./server
	GOOS=linux   GOARCH=arm64 go build -o $(STAGE)/server/plugin-linux-arm64       ./server
	GOOS=darwin  GOARCH=amd64 go build -o $(STAGE)/server/plugin-darwin-amd64      ./server
	GOOS=darwin  GOARCH=arm64 go build -o $(STAGE)/server/plugin-darwin-arm64      ./server
	GOOS=windows GOARCH=amd64 go build -o $(STAGE)/server/plugin-windows-amd64.exe ./server
	go run github.com/kandev/kandev/cmd/plugin-pack -dir $(STAGE) -out $(PKG_OUT)
	rm -rf $(STAGE)
	@echo "Wrote $(PKG_OUT)"

## Host platform only — the fast local iteration loop.
package-host:
	rm -rf $(STAGE)
	mkdir -p $(STAGE)/server $(STAGE)/ui
	cp manifest.yaml $(STAGE)/manifest.yaml
	cp README.md $(STAGE)/README.md
	cp slack-app-manifest.yaml $(STAGE)/slack-app-manifest.yaml
	cp ui/bundle.js $(STAGE)/ui/bundle.js
	go build -o $(STAGE)/server/plugin-$$(go env GOOS)-$$(go env GOARCH)$$(go env GOEXE) ./server
	go run github.com/kandev/kandev/cmd/plugin-pack -dir $(STAGE) -out $(PKG_OUT) -platform-only
	rm -rf $(STAGE)
	@echo "Wrote $(PKG_OUT)"

clean:
	rm -rf bin $(STAGE) kandev-plugin-slack-*.tar.gz
