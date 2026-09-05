package httpstream

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"a2ui/protocol"
	"a2ui/wire"
)

type HandlerFunc func(protocol.Envelope) (protocol.Envelope, *protocol.Error)
type Handler struct {
	max    int
	handle HandlerFunc
}

func NewHandler(maxRecordBytes int, handle HandlerFunc) *Handler {
	if maxRecordBytes < 1 {
		maxRecordBytes = protocol.DefaultLimits().MaxMessageBytes
	}
	return &Handler{max: maxRecordBytes, handle: handle}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.handle == nil {
		http.Error(w, "A2UI handler unavailable", http.StatusServiceUnavailable)
		return
	}

	rd := wire.NewNDJSONReader(r.Body, h.max)
	wr := wire.NewNDJSONWriter(w)
	flusher, _ := w.(http.Flusher)
	wrote := false
	lastSession := ""

	writeStreamError := func(perr *protocol.Error) {
		ev := protocol.Event{V: protocol.Version, Ev: "error", Code: perr.Code, Msg: perr.Message}
		payload, _ := json.Marshal(ev)
		_ = wr.Write(protocol.Envelope{V: protocol.Version, Session: lastSession, Kind: protocol.KindEvent, Payload: payload})
		if flusher != nil {
			flusher.Flush()
		}
	}

	for {
		env, perr, err := rd.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			if !wrote {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else {
				writeStreamError(protocol.NewError("transport.read_failed", err.Error()))
			}
			return
		}
		if perr != nil {
			if !wrote {
				http.Error(w, perr.Error(), http.StatusBadRequest)
			} else {
				writeStreamError(perr)
			}
			return
		}
		lastSession = env.Session

		response, rerr := h.handle(env)
		if rerr != nil {
			if !wrote {
				b, _ := json.Marshal(rerr)
				http.Error(w, string(b), http.StatusUnprocessableEntity)
			} else {
				writeStreamError(rerr)
			}
			return
		}

		if _, err := wire.EncodeEnvelope(response); err != nil {
			if !wrote {
				http.Error(w, "invalid A2UI server response: "+err.Error(), http.StatusInternalServerError)
			} else {
				writeStreamError(protocol.NewError("transport.write_failed", err.Error()))
			}
			return
		}

		if !wrote {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			wrote = true
		}
		if err := wr.Write(response); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}
