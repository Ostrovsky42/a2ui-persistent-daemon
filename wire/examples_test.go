package wire

import (
	"errors"
	"io"
	"os"
	"testing"

	"a2ui/protocol"
)

func TestShippedWireExamplesDecodeStrictly(t *testing.T) {
	for _, path := range []string{"../assets/example-session.ndjson", "../assets/example-events.ndjson"} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		rd := NewNDJSONReader(f, protocol.DefaultLimits().MaxMessageBytes)
		count := 0
		for {
			_, perr, err := rd.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				f.Close()
				t.Fatalf("%s: %v", path, err)
			}
			if perr != nil {
				f.Close()
				t.Fatalf("%s: %v", path, perr)
			}
			count++
		}
		_ = f.Close()
		if count == 0 {
			t.Fatalf("%s: empty example", path)
		}
	}
}
