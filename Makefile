.PHONY: help
## help: prints this help message
help:
	@echo "Usage: \n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: clean
## clean: remove all generated files
clean:
	@echo "Cleaning generated files..."
	@rm -r -f output

.PHONY: build
## build: build the binary
build:
	@go build -o diff

.PHONY: run
## run: build & run the binary
run: build
	@go run .
