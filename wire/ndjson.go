package wire

import (
	"bufio"
	"bytes"
	"errors"
	"io"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

type NDJSONReader struct {
	r   *bufio.Reader
	max int
}

func NewNDJSONReader(rd io.Reader, max int) *NDJSONReader {
	if max < 1 {
		max = 1
	}
	size := 64 * 1024
	if max+1 < size {
		size = max + 1
	}
	if size < 16 {
		size = 16
	}
	return &NDJSONReader{r: bufio.NewReaderSize(rd, size), max: max}
}

func (r *NDJSONReader) Next() (protocol.Envelope, *protocol.Error, error) {
	for {
		line, tooLarge, err := r.readLine()
		if err != nil {
			return protocol.Envelope{}, nil, err
		}
		if tooLarge {
			return protocol.Envelope{}, protocol.NewError("resource.record_too_large", "NDJSON record exceeds configured limit"), nil
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		env, perr := DecodeRecord(line)
		return env, perr, nil
	}
}

func (r *NDJSONReader) readLine() ([]byte, bool, error) {
	buf := make([]byte, 0, minInt(r.max, 4096))
	tooLarge := false
	appendPart := func(part []byte) {
		if tooLarge {
			return
		}
		remaining := r.max - len(buf)
		if len(part) > remaining {
			if remaining > 0 {
				buf = append(buf, part[:remaining]...)
			}
			tooLarge = true
			return
		}
		buf = append(buf, part...)
	}
	for {
		part, err := r.r.ReadSlice('\n')
		if err == nil {
			// Newline (and an optional preceding CR) is framing, not record data.
			part = part[:len(part)-1]
			if len(part) > 0 && part[len(part)-1] == '\r' {
				part = part[:len(part)-1]
			}
			appendPart(part)
			return buf, tooLarge, nil
		}
		appendPart(part)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(part) == 0 && len(buf) == 0 && !tooLarge {
				return nil, false, io.EOF
			}
			return buf, tooLarge, nil
		}
		return nil, false, err
	}
}
func trimLF(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	if len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type NDJSONWriter struct{ w io.Writer }

func NewNDJSONWriter(w io.Writer) *NDJSONWriter { return &NDJSONWriter{w: w} }
func (w *NDJSONWriter) Write(env protocol.Envelope) error {
	b, err := EncodeEnvelope(env)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.w.Write(b)
	return err
}
