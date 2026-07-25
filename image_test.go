package main

import "testing"

func TestParseRef(t *testing.T) {
	type want struct{ name, tag string }
	cases := map[string]want{
		"alpine":       {"library/alpine", "latest"},
		"alpine:3.20":  {"library/alpine", "3.20"},
		"nginx":        {"library/nginx", "latest"},
		"user/repo":    {"user/repo", "latest"},
		"user/repo:v2": {"user/repo", "v2"},
	}
	for in, w := range cases {
		n, tg := parseRef(in)
		if n != w.name || tg != w.tag {
			t.Errorf("parseRef(%q) = (%q,%q), want (%q,%q)", in, n, tg, w.name, w.tag)
		}
	}
}
