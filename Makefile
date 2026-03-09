gmod:
	go mod tidy

bld:
	go build ./...

build:
	go build -o cmd/gophermart/gophermart ./cmd/gophermart

test:
	go test -count 1 ./...

testcov:
	go test -count 1 ./... -cover

vet:
	go vet ./...

vetstat:
	go vet -vettool=$(which statictest) ./...

run:
	go run cmd/gophermart/main.go -a=localhost:8888 -b=ftp://localhost:8383 -l=fatal

