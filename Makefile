.PHONY: run test build clean

run:
	go run cmd/singlenote/main.go

test:
	go test -v ./...

test-e2e:
	cd e2e && npx playwright test

build:
	go build -o singlenote cmd/singlenote/main.go

clean:
	rm -f singlenote singlenote.db

