.PHONY: build

build:
	go build -buildmode=plugin -o script-mastodon.so script.go
