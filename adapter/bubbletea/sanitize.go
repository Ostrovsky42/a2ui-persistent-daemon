package bubbletea

import (
	"strings"
	"unicode/utf8"
)

// SanitizeText removes or neutralizes terminal control sequences, escape sequences,
// C0 control codes (except \n and \t), C1 control codes, and Unicode directional overrides
// from agent-provided text before it reaches layout or terminal rendering.
func SanitizeText(s string) string {
	if s == "" {
		return ""
	}

	var buf strings.Builder
	buf.Grow(len(s))

	const (
		stateNormal = iota
		stateESC
		stateCSI
		stateOSC
		stateOSCESC
		stateString // DCS, APC, PM, SOS
	)

	state := stateNormal
	seqLen := 0

	for i := 0; i < len(s); {
		b := s[i]

		switch state {
		case stateNormal:
			if b == '\r' {
				// Handle CRLF or standalone CR
				if i+1 < len(s) && s[i+1] == '\n' {
					// Skip \r in \r\n, \n will be processed on next iteration
					i++
					continue
				}
				// Standalone CR: replace with newline to avoid cursor line overwrite
				buf.WriteByte('\n')
				i++
				continue
			}

			if b == '\n' || b == '\t' {
				buf.WriteByte(b)
				i++
				continue
			}

			if b == 0x1B { // ESC
				state = stateESC
				seqLen = 1
				i++
				continue
			}

			// Check C0 control codes (0x00 - 0x1F except \t, \n, \r, and 0x7F DEL)
			if b < 0x20 || b == 0x7F {
				i++
				continue
			}

			// Multi-byte UTF-8 decoding
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				// Invalid UTF-8 byte, skip
				i++
				continue
			}

			// Check C1 control codes (U+0080 to U+009F)
			if r >= 0x80 && r <= 0x9F {
				switch r {
				case 0x9B: // 8-bit CSI
					state = stateCSI
					seqLen = 1
				case 0x9D: // 8-bit OSC
					state = stateOSC
					seqLen = 1
				case 0x90, 0x98, 0x9E, 0x9F: // DCS, SOS, PM, APC
					state = stateString
					seqLen = 1
				default:
					// Other C1 control codes: drop
				}
				i += size
				continue
			}

			// Check Trojan Source / Bidi overrides:
			// U+202A..U+202E (LRE, RLE, PDF, LRO, RLO)
			// U+2066..U+2069 (LRI, RLI, FSI, PDI)
			if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
				i += size
				continue
			}

			buf.WriteString(s[i : i+size])
			i += size

		case stateESC:
			seqLen++
			switch b {
			case '[':
				state = stateCSI
			case ']':
				state = stateOSC
			case 'P', '_', '^', 'X': // DCS, APC, PM, SOS
				state = stateString
			case 'N', 'O': // SS2, SS3
				state = stateNormal
			case 0x1B:
				seqLen = 1
			default:
				state = stateNormal
			}
			i++

		case stateCSI:
			seqLen++
			// CSI format: parameter bytes (0x30-0x3F), intermediate bytes (0x20-0x2F), final byte (0x40-0x7E)
			if b >= 0x40 && b <= 0x7E {
				state = stateNormal
			} else if b == 0x1B {
				state = stateESC
				seqLen = 1
			} else if b == '\n' || seqLen > 128 {
				state = stateNormal
				if b == '\n' {
					buf.WriteByte('\n')
				}
			}
			i++

		case stateOSC, stateString:
			seqLen++
			if b == 0x07 { // BEL terminates OSC
				state = stateNormal
			} else if b == 0x1B {
				state = stateOSCESC
			} else if b == '\n' || seqLen > 2048 {
				state = stateNormal
				if b == '\n' {
					buf.WriteByte('\n')
				}
			} else {
				r, size := utf8.DecodeRuneInString(s[i:])
				if r == 0x9C { // C1 ST
					state = stateNormal
					i += size
					continue
				}
			}
			i++

		case stateOSCESC:
			seqLen++
			switch b {
			case '\\': // ST: ESC \
				state = stateNormal
			case '[':
				state = stateCSI
			case ']':
				state = stateOSC
			case 0x1B:
				// consecutive ESC
			default:
				state = stateNormal
			}
			i++
		}
	}

	return buf.String()
}

// SanitizeSingleLineText removes all control/escape sequences and replaces newlines/tabs with spaces.
func SanitizeSingleLineText(s string) string {
	clean := SanitizeText(s)
	if !strings.ContainsAny(clean, "\n\t") {
		return clean
	}
	var b strings.Builder
	b.Grow(len(clean))
	for _, r := range clean {
		if r == '\n' || r == '\t' {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
