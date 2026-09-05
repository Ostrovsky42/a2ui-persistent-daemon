package wire

import "testing"

func FuzzDecodeRecord(f *testing.F) {
	f.Add([]byte(`{"v":1,"seq":1,"op":"commit"}`))
	f.Add([]byte(`{"v":1,"session":"s","kind":"telemetry","seq":1,"payload":{"x":1}}`))
	f.Add([]byte(`{bad}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = DecodeRecord(b)
	})
}
