package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// veth wires a point-to-point link between the host and a container's network
// namespace (host 10.200.1.1 <-> container 10.200.1.2), enables IP forwarding,
// installs a source-NAT (MASQUERADE) rule plus the FORWARD ACCEPTs that let
// routed traffic through, and sets a default route inside the container -- so
// the container reaches the internet, like Docker's default bridge. Shells out
// to iproute2 / iptables; best effort.
type veth struct {
	pid        int
	host, peer string
}

func newVeth(pid int) *veth {
	return &veth{pid: pid, host: "vburrow0", peer: "vburrow1"}
}

const natSubnet = "10.200.1.0/24"

func (v *veth) setup() error {
	p := strconv.Itoa(v.pid)
	_ = exec.Command("ip", "link", "del", v.host).Run() // clear stale link
	steps := [][]string{
		{"ip", "link", "add", v.host, "type", "veth", "peer", "name", v.peer},
		{"ip", "link", "set", v.peer, "netns", p},
		{"ip", "addr", "add", "10.200.1.1/24", "dev", v.host},
		{"ip", "link", "set", v.host, "up"},
		{"nsenter", "-t", p, "-n", "ip", "addr", "add", "10.200.1.2/24", "dev", v.peer},
		{"nsenter", "-t", p, "-n", "ip", "link", "set", v.peer, "up"},
		{"nsenter", "-t", p, "-n", "ip", "link", "set", "lo", "up"},
		{"nsenter", "-t", p, "-n", "ip", "route", "add", "default", "via", "10.200.1.1"},
	}
	for _, s := range steps {
		if out, err := exec.Command(s[0], s[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %v: %s", s, err, out)
		}
	}
	// Outbound internet: forwarding + source-NAT + allow forwarding both ways.
	_ = os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644)
	iptEnsure("nat", "POSTROUTING", "-s", natSubnet, "!", "-o", v.host, "-j", "MASQUERADE")
	iptEnsure("", "FORWARD", "-s", natSubnet, "-j", "ACCEPT")
	iptEnsure("", "FORWARD", "-d", natSubnet, "-j", "ACCEPT")
	return nil
}

func (v *veth) teardown() {
	iptDelete("nat", "POSTROUTING", "-s", natSubnet, "!", "-o", v.host, "-j", "MASQUERADE")
	iptDelete("", "FORWARD", "-s", natSubnet, "-j", "ACCEPT")
	iptDelete("", "FORWARD", "-d", natSubnet, "-j", "ACCEPT")
	_ = exec.Command("ip", "link", "del", v.host).Run()
}

// iptEnsure adds an iptables rule if an identical one isn't already present.
func iptEnsure(table string, rest ...string) {
	base := tableArgs(table)
	check := append(append([]string{}, base...), append([]string{"-C"}, rest...)...)
	if exec.Command("iptables", check...).Run() != nil {
		add := append(append([]string{}, base...), append([]string{"-A"}, rest...)...)
		_ = exec.Command("iptables", add...).Run()
	}
}

func iptDelete(table string, rest ...string) {
	base := tableArgs(table)
	del := append(append([]string{}, base...), append([]string{"-D"}, rest...)...)
	_ = exec.Command("iptables", del...).Run()
}

func tableArgs(table string) []string {
	if table == "" {
		return nil
	}
	return []string{"-t", table}
}
