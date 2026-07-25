package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

// veth wires a point-to-point link between the host and a container's network
// namespace: host end 10.200.1.1, container end 10.200.1.2. It shells out to
// iproute2 (`ip`) and util-linux (`nsenter`) -- the same primitives real tools
// use. Best effort: failures are surfaced as warnings, not fatal.
type veth struct {
	pid        int
	host, peer string
}

func newVeth(pid int) *veth {
	return &veth{pid: pid, host: "vburrow0", peer: "vburrow1"}
}

func (v *veth) setup() error {
	p := strconv.Itoa(v.pid)
	_ = exec.Command("ip", "link", "del", v.host).Run() // clear any stale link
	steps := [][]string{
		{"ip", "link", "add", v.host, "type", "veth", "peer", "name", v.peer},
		{"ip", "link", "set", v.peer, "netns", p},
		{"ip", "addr", "add", "10.200.1.1/24", "dev", v.host},
		{"ip", "link", "set", v.host, "up"},
		{"nsenter", "-t", p, "-n", "ip", "addr", "add", "10.200.1.2/24", "dev", v.peer},
		{"nsenter", "-t", p, "-n", "ip", "link", "set", v.peer, "up"},
		{"nsenter", "-t", p, "-n", "ip", "link", "set", "lo", "up"},
	}
	for _, s := range steps {
		if out, err := exec.Command(s[0], s[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %v: %s", s, err, out)
		}
	}
	return nil
}

func (v *veth) teardown() {
	_ = exec.Command("ip", "link", "del", v.host).Run()
}
