package main

import "testing"

func TestParseMemory(t *testing.T) {
	ok := map[string]int64{
		"64m":     64 << 20,
		"1g":      1 << 30,
		"512k":    512 << 10,
		"1048576": 1048576,
		"  2M ":   2 << 20,
	}
	for in, want := range ok {
		got, err := parseMemory(in)
		if err != nil {
			t.Fatalf("parseMemory(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Errorf("parseMemory(%q) = %d, want %d", in, got, want)
		}
	}
	for _, bad := range []string{"", "xyz", "12mb", "m"} {
		if _, err := parseMemory(bad); err == nil {
			t.Errorf("parseMemory(%q) expected error, got nil", bad)
		}
	}
}
