package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const canonicalModulePath = "github.com/Ostrovsky42/agent-interaction-runtime"

func TestPublicGoModuleUsesCanonicalRepositoryPath(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(data), "\n", 2)[0]
	if firstLine != "module "+canonicalModulePath {
		t.Fatalf("go.mod module line=%q want %q", firstLine, "module "+canonicalModulePath)
	}
}

func TestCurrentPublicProtocolDocsUseAIR1Name(t *testing.T) {
	paths := []string{
		filepath.Join("..", "README.md"),
		filepath.Join("..", "references", "PROTOCOL.md"),
		filepath.Join("..", "docs", "protocol-guide.md"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "AIR/1") {
			t.Errorf("%s does not name the current protocol AIR/1", path)
		}
		for _, legacy := range []string{"A2UI V1", "A2UI v1"} {
			if strings.Contains(text, legacy) {
				t.Errorf("%s still exposes legacy current-protocol name %q", path, legacy)
			}
		}
	}
}
