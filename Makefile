BINARY := burrow

build:
	go build -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

# --- demos (need root for namespaces, except --userns) ---
demo: build            ## isolated shell using host binaries
	sudo ./$(BINARY) run /bin/sh

demo-limits: build     ## 32 MB memory + half a CPU
	sudo ./$(BINARY) run --mem 32m --cpu 0.5 /bin/sh

demo-image: build      ## pull + run a real Alpine image
	sudo ./$(BINARY) run --image alpine /bin/sh

demo-net: build        ## own network namespace + veth (10.200.1.2)
	sudo ./$(BINARY) run --net --image alpine /bin/sh

demo-secure: build     ## Alpine with seccomp + all capabilities dropped
	sudo ./$(BINARY) run --image alpine --seccomp --drop-caps /bin/sh

demo-userns: build     ## rootless: container uid 0 -> your host uid (no sudo)
	./$(BINARY) run --userns /bin/sh

inspect: build         ## live TUI of the most recent running container
	sudo ./$(BINARY) inspect

clean:
	rm -f $(BINARY)

.PHONY: build test vet demo demo-limits demo-image demo-net demo-secure demo-userns inspect clean
