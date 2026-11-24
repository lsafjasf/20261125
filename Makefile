.PHONY: all test vet fmt build demo fixture

all: build test

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fixture:
	go run . gen-scenario --out testdata/scenario.json
	go run . scan testdata/scenario.json --out artifacts/report.json
	go run . scan testdata/scenario.json --format text --out artifacts/report.txt

demo:
	go run . scan testdata/scenario.json --format text
