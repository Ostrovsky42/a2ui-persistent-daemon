package bubbletea

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/document"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func TestSanitizeTextNeutralizesTerminalControls(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text unmodified",
			input:    "Hello, world! 123.",
			expected: "Hello, world! 123.",
		},
		{
			name:     "utf8 and emoji preserved",
			input:    "Привет, мир! 🚀 Проверка: ✓ / ✕",
			expected: "Привет, мир! 🚀 Проверка: ✓ / ✕",
		},
		{
			name:     "newlines and tabs preserved",
			input:    "line1\n\tline2\nline3",
			expected: "line1\n\tline2\nline3",
		},
		{
			name:     "CRLF converted to LF",
			input:    "line1\r\nline2\r\n",
			expected: "line1\nline2\n",
		},
		{
			name:     "standalone CR converted to LF",
			input:    "prompt\roverwritten",
			expected: "prompt\noverwritten",
		},
		{
			name:     "OSC 52 clipboard injection with BEL stripped",
			input:    "Click \x1b]52;c;c2VjcmV0\x07here",
			expected: "Click here",
		},
		{
			name:     "OSC 52 clipboard injection with ST stripped",
			input:    "Click \x1b]52;c;c2VjcmV0\x1b\\here",
			expected: "Click here",
		},
		{
			name:     "OSC 8 hyperlink stripped keeping visible text",
			input:    "Visit \x1b]8;;https://evil.com/phish\x1b\\benign link\x1b]8;;\x1b\\ now",
			expected: "Visit benign link now",
		},
		{
			name:     "OSC 0 window title injection stripped",
			input:    "Status: \x1b]0;Pwned Title\x07ready",
			expected: "Status: ready",
		},
		{
			name:     "CSI clear screen stripped",
			input:    "Before \x1b[2J\x1b[HAfter",
			expected: "Before After",
		},
		{
			name:     "CSI color escapes stripped leaving pure text",
			input:    "\x1b[31;1mRed Alert\x1b[0m",
			expected: "Red Alert",
		},
		{
			name:     "C1 8-bit CSI stripped",
			input:    "C1 test: \xc2\x9b2Jclean",
			expected: "C1 test: clean",
		},
		{
			name:     "C1 8-bit OSC stripped",
			input:    "C1 OSC: \xc2\x9d52;c;secret\xc2\x9cclean",
			expected: "C1 OSC: clean",
		},
		{
			name:     "C0 control characters (NUL, BEL, BS, DEL) dropped",
			input:    "bad\x00data\x07with\x08control\x7fchars",
			expected: "baddatawithcontrolchars",
		},
		{
			name:     "Bidi override (RLO/PDF) dropped",
			input:    "Safe text \u202ereversed\u202c end",
			expected: "Safe text reversed end",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeText(tc.input)
			if got != tc.expected {
				t.Fatalf("SanitizeText(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSanitizeSingleLineText(t *testing.T) {
	input := "Line 1\r\n\tLine \x1b[32m2\x1b[0m\nLine 3"
	got := SanitizeSingleLineText(input)
	if strings.ContainsAny(got, "\r\n\t\x1b") {
		t.Fatalf("SanitizeSingleLineText contained illegal bytes: %q", got)
	}
	expected := "Line 1  Line 2 Line 3"
	if got != expected {
		t.Fatalf("SanitizeSingleLineText(%q) = %q; want %q", input, got, expected)
	}
}

func TestRendererSanitizesInjectedDocumentNodes(t *testing.T) {
	theme := DefaultTheme
	renderer := NewRendererWithPreset(theme, PresetMinimal)

	doc := document.Document{
		Revision: 1,
		Nodes: map[string]document.Node{
			"root": {
				ID:       "root",
				Type:     protocol.NodeBox,
				Children: []string{"mal-text", "mal-input", "mal-actions", "mal-table"},
			},
			"mal-text": {
				ID:     "mal-text",
				Type:   protocol.NodeText,
				Parent: "root",
				Text:   "Injected: \x1b]52;c;c2VjcmV0\x07EndText",
				Props: map[string]json.RawMessage{
					"variant": json.RawMessage(`"title"`),
				},
			},
			"mal-input": {
				ID:     "mal-input",
				Type:   protocol.NodeInput,
				Parent: "root",
				Props: map[string]json.RawMessage{
					"placeholder": json.RawMessage(`"\u001b]8;;http://evil.com\u001b\\Phish\u001b]8;;\u001b\\"`),
				},
			},
			"mal-actions": {
				ID:     "mal-actions",
				Type:   protocol.NodeActions,
				Parent: "root",
				Props: map[string]json.RawMessage{
					"items": json.RawMessage(`[{"key":"\u001b[2Jx","label":"\u001b]0;HackedTitle\u0007Click"}]`),
				},
			},
			"mal-table": {
				ID:     "mal-table",
				Type:   protocol.NodeTable,
				Parent: "root",
				Props: map[string]json.RawMessage{
					"columns": json.RawMessage(`[{"title":"\u001b[31;1mCol1\u001b[0m","width":10}]`),
					"rows":    json.RawMessage(`[["\u009b2JCell1"]]`),
				},
			},
		},
	}

	result := renderer.RenderFrame(
		doc,
		"mal-input",
		map[string]string{"mal-input": "\x1b[2Juser-input"},
		nil,
		InteractionState{},
		80,
		24,
		RenderState{CursorVisible: false},
	)

	frame := result.Frame

	// Verify dangerous escape sequences are nowhere in the output frame
	for _, dangerous := range []string{
		"]52;",     // OSC 52 clipboard
		"]8;;",     // OSC 8 hyperlink
		"]0;",      // OSC 0 window title
		"[2J",      // CSI clear screen
		"\xc2\x9b", // 8-bit CSI
		"\xc2\x9d", // 8-bit OSC
		"\x07",     // BEL
	} {
		if strings.Contains(frame, dangerous) {
			t.Errorf("rendered frame contains dangerous sequence %q: %q", dangerous, frame)
		}
	}

	// Verify sanitized text content is present
	for _, expectedText := range []string{
		"Injected: EndText",
		"user-input",
		"x",
		"Click",
		"Col1",
		"Cell1",
	} {
		if !strings.Contains(frame, expectedText) {
			t.Errorf("rendered frame missing expected sanitized text %q", expectedText)
		}
	}
}

func TestSanitizeSplitSequenceAcrossAppends(t *testing.T) {
	theme := DefaultTheme
	renderer := NewRendererWithPreset(theme, PresetMinimal)

	doc := document.Document{
		Revision: 1,
		Nodes: map[string]document.Node{
			"root": {
				ID:       "root",
				Type:     protocol.NodeBox,
				Children: []string{"split-text"},
			},
			"split-text": {
				ID:     "split-text",
				Type:   protocol.NodeText,
				Parent: "root",
				Text:   "2JCleanText",
				Props: map[string]json.RawMessage{
					"text": json.RawMessage(`"\u001b["`),
				},
			},
		},
	}

	result := renderer.RenderFrame(doc, "", nil, nil, InteractionState{}, 80, 24, RenderState{})
	if strings.Contains(result.Frame, "[2J") {
		t.Fatalf("split sequence escaped into CSI [2J: %q", result.Frame)
	}
	if !strings.Contains(result.Frame, "CleanText") {
		t.Fatalf("expected text CleanText missing: %q", result.Frame)
	}
}
