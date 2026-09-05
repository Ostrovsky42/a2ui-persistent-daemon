package ipc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"a2ui/wire"
)

const DefaultMaxMessageBytes = 16 << 20

func DecodeMessage(raw []byte) (Message, *Error) {
	var msg Message
	if perr := wire.StrictUnmarshal(raw, &msg); perr != nil {
		return Message{}, NewError("ipc.invalid_message", perr.Message)
	}
	if err := validateMessage(msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func EncodeMessage(msg Message) ([]byte, error) {
	if perr := validateMessage(msg); perr != nil {
		return nil, perr
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if _, perr := DecodeMessage(b); perr != nil {
		return nil, perr
	}
	return b, nil
}

type Reader struct {
	r   *bufio.Reader
	max int
}

func NewReader(r io.Reader, maxRecordBytes int) *Reader {
	if maxRecordBytes < 1 {
		maxRecordBytes = DefaultMaxMessageBytes
	}
	return &Reader{r: bufio.NewReaderSize(r, minInt(maxRecordBytes+1, 64<<10)), max: maxRecordBytes}
}

func (r *Reader) Next() (Message, *Error, error) {
	record, tooLarge, err := r.readRecord()
	if err != nil {
		return Message{}, nil, err
	}
	if tooLarge {
		return Message{}, NewError("ipc.message_too_large", fmt.Sprintf("IPC record exceeds %d bytes", r.max)), nil
	}
	msg, perr := DecodeMessage(record)
	return msg, perr, nil
}

func (r *Reader) readRecord() ([]byte, bool, error) {
	var buf []byte
	tooLarge := false
	appendPart := func(part []byte) {
		if tooLarge {
			return
		}
		if len(buf)+len(part) > r.max {
			remaining := r.max + 1 - len(buf)
			if remaining > 0 {
				buf = append(buf, part[:minInt(remaining, len(part))]...)
			}
			tooLarge = true
			return
		}
		buf = append(buf, part...)
	}
	for {
		part, err := r.r.ReadSlice('\n')
		if err == nil {
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

type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

func (w *Writer) Write(msg Message) error {
	b, err := EncodeMessage(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.w.Write(b)
	return err
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// strictDecode is retained for fuzz/debug helpers where mapping through the
// public wire package would hide the underlying JSON error.
func strictDecode(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
