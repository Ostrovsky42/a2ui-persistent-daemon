package supervisor

import "testing"

func envLookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestViewerPolicyParsing(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  ViewerPolicy
	}{
		{"", ViewerAuto},
		{"auto", ViewerAuto},
		{"never", ViewerNever},
		{"always", ViewerAlways},
	} {
		got, err := ParseViewerPolicy(tc.input)
		if err != nil {
			t.Fatalf("ParseViewerPolicy(%q): %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ParseViewerPolicy(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
	if _, err := ParseViewerPolicy("sometimes"); err == nil {
		t.Fatal("ParseViewerPolicy accepted invalid viewer policy")
	}
}

func TestAutoPolicySuppressesCI(t *testing.T) {
	if ViewerAuto.Allows(envLookup(map[string]string{"CI": "true", "DISPLAY": ":0"})) {
		t.Fatal("auto policy allowed launch in CI")
	}
}

func TestAutoPolicySuppressesSSH(t *testing.T) {
	for _, key := range []string{"SSH_CONNECTION", "SSH_TTY"} {
		if ViewerAuto.Allows(envLookup(map[string]string{key: "present", "DISPLAY": ":0"})) {
			t.Fatalf("auto policy allowed launch with %s", key)
		}
	}
}

func TestAutoPolicySuppressesHeadless(t *testing.T) {
	if ViewerAuto.Allows(envLookup(nil)) {
		t.Fatal("auto policy allowed launch without DISPLAY or WAYLAND_DISPLAY")
	}
}

func TestAutoPolicyAllowsGraphicalLocalSession(t *testing.T) {
	if !ViewerAuto.Allows(envLookup(map[string]string{"WAYLAND_DISPLAY": "wayland-0"})) {
		t.Fatal("auto policy suppressed graphical local session")
	}
}

func TestNeverAndAlwaysAreExplicit(t *testing.T) {
	headlessCI := envLookup(map[string]string{"CI": "1", "SSH_TTY": "/dev/pts/1"})
	if ViewerNever.Allows(headlessCI) {
		t.Fatal("never policy allowed launch")
	}
	if !ViewerAlways.Allows(headlessCI) {
		t.Fatal("always policy did not override environment suppression")
	}
}
