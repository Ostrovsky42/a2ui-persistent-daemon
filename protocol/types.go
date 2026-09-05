package protocol

import "encoding/json"

const Version = 1

type OpType string

const (
	OpUpsert OpType = "upsert"
	OpProps  OpType = "props"
	OpText   OpType = "text"
	OpRemove OpType = "remove"
	OpFocus  OpType = "focus"
	OpCommit OpType = "commit"
)

type NodeType string

const (
	NodeBox      NodeType = "box"
	NodeText     NodeType = "text"
	NodeViewport NodeType = "viewport"
	NodeTable    NodeType = "table"
	NodeInput    NodeType = "input"
	NodeActions  NodeType = "actions"
	NodeProgress NodeType = "progress"
)

func (n NodeType) Valid() bool {
	switch n {
	case NodeBox, NodeText, NodeViewport, NodeTable, NodeInput, NodeActions, NodeProgress:
		return true
	}
	return false
}
func (n NodeType) Container() bool { return n == NodeBox || n == NodeViewport }

type Operation struct {
	V      int             `json:"v"`
	Seq    int64           `json:"seq"`
	Op     OpType          `json:"op"`
	ID     string          `json:"id,omitempty"`
	Type   NodeType        `json:"type,omitempty"`
	Parent string          `json:"parent,omitempty"`
	Index  *int            `json:"index,omitempty"`
	Text   string          `json:"text,omitempty"`
	Frame  string          `json:"frame,omitempty"`
	Props  json.RawMessage `json:"props,omitempty"`
}

type EnvelopeKind string

const (
	KindHello     EnvelopeKind = "hello"
	KindHelloAck  EnvelopeKind = "hello_ack"
	KindOperation EnvelopeKind = "operation"
	KindEvent     EnvelopeKind = "event"
	KindTelemetry EnvelopeKind = "telemetry"
)

type Envelope struct {
	V       int             `json:"v"`
	Session string          `json:"session,omitempty"`
	Kind    EnvelopeKind    `json:"kind"`
	Seq     uint64          `json:"seq"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Event struct {
	V            int             `json:"v"`
	Seq          uint64          `json:"seq"`
	Ev           string          `json:"ev"`
	ID           string          `json:"id,omitempty"`
	Code         string          `json:"code,omitempty"`
	Msg          string          `json:"msg,omitempty"`
	Action       string          `json:"action,omitempty"`
	InvocationID string          `json:"invocation_id,omitempty"`
	Value        string          `json:"value,omitempty"`
	Row          int             `json:"row"`
	RowID        string          `json:"row_id,omitempty"`
	W            int             `json:"w,omitempty"`
	H            int             `json:"h,omitempty"`
	Revision     uint64          `json:"revision,omitempty"`
	RelatedSeq   uint64          `json:"related_seq,omitempty"`
	ThroughSeq   uint64          `json:"through_seq,omitempty"`
	Frame        string          `json:"frame,omitempty"`
	Args         json.RawMessage `json:"args,omitempty"`
}

type Limits struct {
	MaxMessageBytes     int `json:"max_message_bytes"`
	MaxDocumentBytes    int `json:"max_document_bytes"`
	MaxNodes            int `json:"max_nodes"`
	MaxDepth            int `json:"max_depth"`
	MaxChildren         int `json:"max_children"`
	MaxTextBytesPerNode int `json:"max_text_bytes_per_node"`
	MaxTotalTextBytes   int `json:"max_total_text_bytes"`
	MaxTableRows        int `json:"max_table_rows"`
	MaxTableColumns     int `json:"max_table_columns"`
	MaxPendingEvents    int `json:"max_pending_events"`
	MaxPendingMutations int `json:"max_pending_mutations"`
	MaxInflightActions  int `json:"max_inflight_actions"`
	MaxDatagramBytes    int `json:"max_datagram_bytes"`
}

func DefaultLimits() Limits {
	return Limits{MaxMessageBytes: 1 << 20, MaxDocumentBytes: 8 << 20, MaxNodes: 2048, MaxDepth: 32, MaxChildren: 512, MaxTextBytesPerNode: 256 << 10, MaxTotalTextBytes: 2 << 20, MaxTableRows: 4096, MaxTableColumns: 64, MaxPendingEvents: 256, MaxPendingMutations: 256, MaxInflightActions: 32, MaxDatagramBytes: 1200}
}

type MutationPolicy struct{ ForceInputValue bool }

type Hello struct {
	Versions []int    `json:"versions"`
	Features []string `json:"features,omitempty"`
}
type HelloAck struct {
	Version    int        `json:"version"`
	Features   []string   `json:"features"`
	Components []NodeType `json:"components"`
	Limits     Limits     `json:"limits"`
}

func AllNodeTypes() []NodeType {
	return []NodeType{NodeBox, NodeText, NodeViewport, NodeTable, NodeInput, NodeActions, NodeProgress}
}

// EffectiveLimits replaces non-positive configured values with finite defaults.
// Explicit positive values are preserved. This is used by local runtimes before
// advertising or enforcing limits; peer-provided limits are validated instead.
func EffectiveLimits(l Limits) Limits {
	d := DefaultLimits()
	if l.MaxMessageBytes < 1 {
		l.MaxMessageBytes = d.MaxMessageBytes
	}
	if l.MaxDocumentBytes < 1 {
		l.MaxDocumentBytes = d.MaxDocumentBytes
	}
	if l.MaxNodes < 1 {
		l.MaxNodes = d.MaxNodes
	}
	if l.MaxDepth < 1 {
		l.MaxDepth = d.MaxDepth
	}
	if l.MaxChildren < 1 {
		l.MaxChildren = d.MaxChildren
	}
	if l.MaxTextBytesPerNode < 1 {
		l.MaxTextBytesPerNode = d.MaxTextBytesPerNode
	}
	if l.MaxTotalTextBytes < 1 {
		l.MaxTotalTextBytes = d.MaxTotalTextBytes
	}
	if l.MaxTableRows < 1 {
		l.MaxTableRows = d.MaxTableRows
	}
	if l.MaxTableColumns < 1 {
		l.MaxTableColumns = d.MaxTableColumns
	}
	if l.MaxPendingEvents < 1 {
		l.MaxPendingEvents = d.MaxPendingEvents
	}
	if l.MaxPendingMutations < 1 {
		l.MaxPendingMutations = d.MaxPendingMutations
	}
	if l.MaxInflightActions < 1 {
		l.MaxInflightActions = d.MaxInflightActions
	}
	if l.MaxDatagramBytes < 1 {
		l.MaxDatagramBytes = d.MaxDatagramBytes
	}
	return l
}
