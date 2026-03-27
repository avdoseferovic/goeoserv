.PHONY: build run test docker-build clean

build:
	go build -o bin/goeoserv ./cmd/goeoserv

run: build
	./bin/goeoserv

test:
	go test ./...

docker-build:
	docker build -t goeoserv:local .

clean:
	rm -rf bin/
