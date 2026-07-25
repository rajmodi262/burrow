BINARY := burrow

build:
	go build -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

demo: build            ## real Alpine shell
	sudo ./$(BINARY) run --image alpine /bin/sh

demo-hello: build      ## run hello-world via its image entrypoint
	sudo ./$(BINARY) run --image hello-world

demo-limits: build     ## 32 MB memory + half a CPU
	sudo ./$(BINARY) run --image alpine --mem 32m --cpu 0.5 /bin/sh

demo-net: build        ## container with internet (NAT + DNS)
	sudo ./$(BINARY) run --image alpine --net /bin/sh

demo-secure: build     ## seccomp + all capabilities dropped
	sudo ./$(BINARY) run --image alpine --seccomp --drop-caps /bin/sh

demo-userns: build     ## rootless: container uid 0 -> host uid (no sudo)
	./$(BINARY) run --userns /bin/sh

ps: build
	sudo ./$(BINARY) ps

images: build
	./$(BINARY) images

inspect: build
	sudo ./$(BINARY) inspect

clean:
	rm -f $(BINARY)

.PHONY: build test vet demo demo-hello demo-limits demo-net demo-secure demo-userns ps images inspect clean
