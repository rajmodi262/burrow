BINARY := burrow

build:
	go build -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

# Demo: isolated shell using the host's binaries (no rootfs needed).
demo: build
	sudo ./$(BINARY) run /bin/sh

# Demo with a 16 MB memory cap applied via cgroups v2.
demo-mem: build
	sudo ./$(BINARY) run --mem 16m /bin/sh

clean:
	rm -f $(BINARY)

.PHONY: build test vet demo demo-mem clean
