BINARY := burrow

build:
	go build -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

# --- demos (need root for namespaces) ---
demo: build            ## isolated shell using host binaries
	sudo ./$(BINARY) run /bin/sh

demo-limits: build     ## 32 MB memory + half a CPU
	sudo ./$(BINARY) run --mem 32m --cpu 0.5 /bin/sh

demo-net: build        ## own network namespace + veth (10.200.1.2)
	sudo ./$(BINARY) run --net /bin/sh

rootfs:                ## build a tiny busybox rootfs into ./rootfs
	./get-rootfs.sh

demo-rootfs: build rootfs  ## busybox shell in an overlayfs rootfs
	sudo ./$(BINARY) run --rootfs ./rootfs /bin/sh

inspect: build         ## live TUI of the most recent running container
	sudo ./$(BINARY) inspect

clean:
	rm -f $(BINARY)

.PHONY: build test vet demo demo-limits demo-net rootfs demo-rootfs inspect clean
