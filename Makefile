.PHONY: build run test clean

build:
	go build -o bin/goeoserv ./cmd/goeoserv

run: build
	./bin/goeoserv

test:
	go test ./...

clean:
	rm -rf bin/
