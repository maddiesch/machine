ROOT_DIR := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

machine.pb.go:
	protoc -I=${ROOT_DIR}/ --go_out=${ROOT_DIR}/ ${ROOT_DIR}/machine.proto

.PHONY: build
build: machine.pb.go

.PHONY: clean
clean:
	cd ${ROOT_DIR} && go clean
	-rm -r ${ROOT_DIR}/*.pb.go

.PHONY: test
test: clean build
	cd ${ROOT_DIR} && go test -tags debug -v ./...
