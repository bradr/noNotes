.PHONY: run test build clean

run:
	go run cmd/nonotes/main.go

test:
	go test -v ./...

test-e2e:
	cd e2e && npx playwright test

build:
	go build -o nonotes cmd/nonotes/main.go

clean:
	rm -f nonotes nonotes.db

