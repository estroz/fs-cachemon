
all: fmt test

fmt:
	go fmt ./...

test:
	go test -race ./...
