//go:build unix

package main

import "testing"

func TestValidateWebListenAddressAllowsOnlyNumericLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "127.0.0.1:8787", "[::1]:0"} {
		if err := validateWebListenAddress(addr); err != nil {
			t.Fatalf("validate %q: %v", addr, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:8787", "192.168.1.20:8787", "localhost:8787", ":8787", "127.0.0.1"} {
		if err := validateWebListenAddress(addr); err == nil {
			t.Fatalf("validate %q unexpectedly succeeded", addr)
		}
	}
}

func TestWebRendererURLFormatsListenerAddress(t *testing.T) {
	if got := webRendererURL("127.0.0.1:43123"); got != "http://127.0.0.1:43123/" {
		t.Fatalf("url=%q", got)
	}
	if got := webRendererURL("[::1]:43123"); got != "http://[::1]:43123/" {
		t.Fatalf("ipv6 url=%q", got)
	}
}
