package main

import "testing"

func TestParseMemory(t *testing.T) {
	ok := map[string]int64{
		"64m": 64 << 20, "1g": 1 << 30, "512k": 512 << 10,
		"1048576": 1048576, "  2M ": 2 << 20,
	}
	for in, want := range ok {
		got, err := parseMemory(in)
		if err != nil {
			t.Fatalf("parseMemory(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("parseMemory(%q) = %d, want %d", in, got, want)
		}
	}
	for _, bad := range []string{"", "xyz", "12mb", "m"} {
		if _, err := parseMemory(bad); err == nil {
			t.Errorf("parseMemory(%q): expected error", bad)
		}
	}
}

func TestParseCPU(t *testing.T) {
	ok := map[string]string{
		"0.5": "50000 100000", "1": "100000 100000", "2": "200000 100000",
	}
	for in, want := range ok {
		got, err := parseCPU(in)
		if err != nil {
			t.Fatalf("parseCPU(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("parseCPU(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "x", "-1", "0"} {
		if _, err := parseCPU(bad); err == nil {
			t.Errorf("parseCPU(%q): expected error", bad)
		}
	}
}

func TestHuman(t *testing.T) {
	cases := map[string]string{
		"": "-", "max": "max", "1048576": "1.0 MiB", "1073741824": "1.0 GiB", "512": "512 B",
	}
	for in, want := range cases {
		if got := human(in); got != want {
			t.Errorf("human(%q) = %q, want %q", in, got, want)
		}
	}
}
