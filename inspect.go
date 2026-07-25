package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var nsList = []string{"uts", "pid", "mnt", "net", "ipc", "user", "cgroup"}

type snapshot struct {
	pid              int
	comm             string
	hostname         string
	ns, hostNS       map[string]string
	cgPath           string
	memMax, memCur   string
	cpuMax, cpuUsec  string
	mounts           []string
	err              error
}

func inspect(args []string) {
	once := false
	pid := 0
	for _, a := range args {
		if a == "--once" {
			once = true
		} else {
			fmt.Sscanf(a, "%d", &pid)
		}
	}
	if pid == 0 {
		pid = discoverContainer()
	}
	if pid == 0 {
		fmt.Fprintln(os.Stderr, "burrow: no running container found (pass a PID, or start one with `burrow run`)")
		os.Exit(1)
	}
	if once {
		s := gather(pid)
		if s.err != nil {
			fmt.Fprintln(os.Stderr, "burrow inspect:", s.err)
			os.Exit(1)
		}
		fmt.Print(s.render())
		return
	}
	runTUI(pid)
}

// discoverContainer returns the pid of the most recent live burrow container,
// found via its cgroup directory name (burrow.<pid>).
func discoverContainer() int {
	matches, _ := filepath.Glob(filepath.Join(cgroupRoot, "burrow.*"))
	best := 0
	for _, m := range matches {
		var p int
		if _, err := fmt.Sscanf(filepath.Base(m), "burrow.%d", &p); err != nil || p == 0 {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", p)); err == nil && p > best {
			best = p
		}
	}
	return best
}

func gather(pid int) snapshot {
	s := snapshot{pid: pid, ns: map[string]string{}, hostNS: map[string]string{}}
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		s.err = fmt.Errorf("no such pid %d", pid)
		return s
	}
	s.comm = readFile(fmt.Sprintf("/proc/%d/comm", pid))
	self := os.Getpid()
	for _, n := range nsList {
		s.ns[n] = nsID(pid, n)
		s.hostNS[n] = nsID(self, n)
	}
	if out, err := exec.Command("nsenter", "-t", fmt.Sprint(pid), "-u", "hostname").Output(); err == nil {
		s.hostname = strings.TrimSpace(string(out))
	}
	cg := readFile(fmt.Sprintf("/proc/%d/cgroup", pid)) // "0::/burrow.123"
	if i := strings.LastIndex(cg, "::"); i >= 0 {
		s.cgPath = cg[i+2:]
	}
	base := filepath.Join(cgroupRoot, strings.TrimPrefix(s.cgPath, "/"))
	s.memMax = readFile(filepath.Join(base, "memory.max"))
	s.memCur = readFile(filepath.Join(base, "memory.current"))
	s.cpuMax = readFile(filepath.Join(base, "cpu.max"))
	for _, ln := range strings.Split(readFile(filepath.Join(base, "cpu.stat")), "\n") {
		if strings.HasPrefix(ln, "usage_usec") {
			s.cpuUsec = strings.TrimSpace(strings.TrimPrefix(ln, "usage_usec"))
		}
	}
	for _, ln := range strings.Split(readFile(fmt.Sprintf("/proc/%d/mountinfo", pid)), "\n") {
		if f := strings.Fields(ln); len(f) >= 5 {
			s.mounts = append(s.mounts, f[4])
		}
	}
	sort.Strings(s.mounts)
	return s
}

func (s snapshot) render() string {
	key := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	iso := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	shared := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))

	var b strings.Builder
	fmt.Fprintf(&b, "%s  pid=%d  comm=%s  hostname=%s\n\n",
		key.Render("container"), s.pid, orDash(s.comm), orDash(s.hostname))

	b.WriteString(key.Render("namespaces") + "  (isolated = different inode than host)\n")
	for _, n := range nsList {
		status := shared.Render("shared")
		if s.ns[n] != s.hostNS[n] && s.ns[n] != "-" {
			status = iso.Render("ISOLATED")
		}
		fmt.Fprintf(&b, "  %-7s %-24s %s\n", n, s.ns[n], status)
	}

	fmt.Fprintf(&b, "\n%s  %s\n", key.Render("cgroup"), orDash(s.cgPath))
	fmt.Fprintf(&b, "  memory.current = %-10s memory.max = %s\n", human(s.memCur), human(s.memMax))
	fmt.Fprintf(&b, "  cpu.max        = %-10s cpu.usage  = %s us\n", orDash(s.cpuMax), orDash(s.cpuUsec))

	fmt.Fprintf(&b, "\n%s  %d total (showing up to 6)\n", key.Render("mounts"), len(s.mounts))
	for i, mp := range s.mounts {
		if i >= 6 {
			break
		}
		fmt.Fprintf(&b, "  %s\n", mp)
	}
	return b.String()
}

func nsID(pid int, ns string) string {
	l, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/%s", pid, ns))
	if err != nil {
		return "-"
	}
	return l
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func human(s string) string {
	if s == "" {
		return "-"
	}
	if s == "max" {
		return "max"
	}
	var n int64
	if _, err := fmt.Sscan(s, &n); err != nil {
		return s
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
