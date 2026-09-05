package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type rawProps map[string]json.RawMessage

var allowedProps = map[NodeType]map[string]bool{
	NodeBox:      {"dir": true, "padding": true, "gap": true, "border": true, "style": true, "variant": true, "align": true, "responsive": true, "flex": true},
	NodeText:     {"text": true, "style": true, "variant": true, "flex": true},
	NodeViewport: {"height": true, "wrap": true, "follow_tail": true, "scrollable": true, "flex": true},
	NodeTable:    {"columns": true, "rows": true, "selectable": true, "row_ids": true, "action": true, "variant": true, "flex": true},
	NodeInput:    {"placeholder": true, "value": true, "force": true, "action": true, "flex": true},
	NodeActions:  {"items": true, "variant": true, "flex": true},
	NodeProgress: {"value": true, "label": true, "variant": true, "state": true, "flex": true},
}

func defaults(t NodeType) rawProps {
	src := map[NodeType]string{
		NodeBox:      `{"dir":"col","padding":0,"gap":0,"border":"none","style":{},"variant":"plain","align":"start","responsive":"none"}`,
		NodeText:     `{"text":"","style":{},"variant":"body"}`,
		NodeViewport: `{"height":10,"wrap":true,"follow_tail":false,"scrollable":false}`,
		NodeTable:    `{"columns":[],"rows":[],"selectable":false,"action":"","variant":"normal"}`,
		NodeInput:    `{"placeholder":"","value":"","action":""}`,
		NodeActions:  `{"items":[],"variant":"inline"}`,
		NodeProgress: `{"value":0,"label":"","variant":"bar","state":"normal"}`,
	}[t]
	var m rawProps
	_ = json.Unmarshal([]byte(src), &m)
	return m
}

func cloneProps(in rawProps) rawProps {
	out := make(rawProps, len(in))
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

func NormalizeProps(t NodeType, raw json.RawMessage, full bool) (map[string]json.RawMessage, MutationPolicy, *Error) {
	if !t.Valid() {
		return nil, MutationPolicy{}, NewError("schema.unknown_type", fmt.Sprintf("unknown node type %q", t))
	}
	patch := rawProps{}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) != 0 {
		if bytes.Equal(trimmed, []byte("null")) {
			return nil, MutationPolicy{}, NewError("schema.invalid_props", "props must be an object")
		}
		if err := json.Unmarshal(trimmed, &patch); err != nil || patch == nil {
			if err != nil {
				return nil, MutationPolicy{}, NewError("schema.invalid_props", err.Error())
			}
			return nil, MutationPolicy{}, NewError("schema.invalid_props", "props must be an object")
		}
	}
	for k := range patch {
		if !allowedProps[t][k] {
			e := NewError("schema.unknown_property", fmt.Sprintf("property %q is not allowed for %s", k, t))
			e.Path = "/props/" + k
			return nil, MutationPolicy{}, e
		}
	}
	policy := MutationPolicy{}
	if t == NodeInput {
		if v, ok := patch["force"]; ok {
			if err := json.Unmarshal(v, &policy.ForceInputValue); err != nil {
				return nil, policy, NewError("schema.invalid_props", "input.force must be boolean")
			}
			delete(patch, "force")
		}
	}
	if full {
		d := defaults(t)
		for k, v := range patch {
			d[k] = v
		}
		patch = d
	}
	if err := validateNormalized(t, patch); err != nil {
		return nil, policy, err
	}
	return map[string]json.RawMessage(patch), policy, nil
}

func validateNormalized(t NodeType, p rawProps) *Error {
	bad := func(path, msg string) *Error { e := NewError("schema.invalid_props", msg); e.Path = path; return e }
	intField := func(name string, min int) (int, *Error) {
		v, ok := p[name]
		if !ok {
			return 0, nil
		}
		var n int
		if json.Unmarshal(v, &n) != nil || n < min {
			return 0, bad("/props/"+name, fmt.Sprintf("%s must be integer >= %d", name, min))
		}
		return n, nil
	}
	strField := func(name string) (string, *Error) {
		v, ok := p[name]
		if !ok {
			return "", nil
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			return "", bad("/props/"+name, name+" must be string")
		}
		return s, nil
	}
	boolField := func(name string) *Error {
		v, ok := p[name]
		if !ok {
			return nil
		}
		var b bool
		if json.Unmarshal(v, &b) != nil {
			return bad("/props/"+name, name+" must be boolean")
		}
		return nil
	}
	validateEnum := func(name string, allowed ...string) *Error {
		s, e := strField(name)
		if e != nil {
			return e
		}
		if s == "" {
			return nil
		}
		for _, v := range allowed {
			if s == v {
				return nil
			}
		}
		return bad("/props/"+name, fmt.Sprintf("invalid %s", name))
	}
	validateFlex := func(raw json.RawMessage) *Error {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil || m == nil {
			return bad("/props/flex", "flex must be object")
		}
		vals := map[string]int{}
		for k, v := range m {
			switch k {
			case "grow", "basis":
				var n int
				if json.Unmarshal(v, &n) != nil || n < 0 {
					return bad("/props/flex/"+k, k+" must be integer >= 0")
				}
				vals[k] = n
			case "min_width", "max_width":
				var n int
				if json.Unmarshal(v, &n) != nil || n < 1 {
					return bad("/props/flex/"+k, k+" must be integer >= 1")
				}
				vals[k] = n
			default:
				e := NewError("schema.unknown_property", fmt.Sprintf("unknown flex property %q", k))
				e.Path = "/props/flex/" + k
				return e
			}
		}
		if min, ok := vals["min_width"]; ok {
			if max, ok2 := vals["max_width"]; ok2 && max < min {
				return bad("/props/flex/max_width", "max_width must be >= min_width")
			}
		}
		return nil
	}
	validateStyle := func(raw json.RawMessage) *Error {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil {
			return bad("/props/style", "style must be object")
		}
		for k, v := range m {
			switch k {
			case "fg", "bg":
				var s string
				if json.Unmarshal(v, &s) != nil {
					return bad("/props/style/"+k, k+" must be string")
				}
				if s != "" && s != "primary" && s != "warn" && s != "error" && s != "muted" {
					e := NewError("schema.invalid_color", fmt.Sprintf("unsupported color %q", s))
					e.Path = "/props/style/" + k
					return e
				}
			case "bold", "dim":
				var b bool
				if json.Unmarshal(v, &b) != nil {
					return bad("/props/style/"+k, k+" must be boolean")
				}
			default:
				return func() *Error {
					e := NewError("schema.unknown_property", fmt.Sprintf("unknown style property %q", k))
					e.Path = "/props/style/" + k
					return e
				}()
			}
		}
		return nil
	}
	if v, ok := p["flex"]; ok {
		if e := validateFlex(v); e != nil {
			return e
		}
	}
	switch t {
	case NodeBox:
		if e := validateEnum("variant", "plain", "panel", "card", "section"); e != nil {
			return e
		}
		if e := validateEnum("align", "start", "center", "end", "stretch"); e != nil {
			return e
		}
		if e := validateEnum("responsive", "none", "stack"); e != nil {
			return e
		}
		dir, e := strField("dir")
		if e != nil {
			return e
		}
		if dir != "" && dir != "col" && dir != "row" {
			return bad("/props/dir", "dir must be col or row")
		}
		if _, e = intField("padding", 0); e != nil {
			return e
		}
		if _, e = intField("gap", 0); e != nil {
			return e
		}
		border, e := strField("border")
		if e != nil {
			return e
		}
		if border != "" && border != "none" && border != "rounded" && border != "normal" {
			return bad("/props/border", "invalid border")
		}
		if v, ok := p["style"]; ok {
			if e := validateStyle(v); e != nil {
				return e
			}
		}
	case NodeText:
		if e := validateEnum("variant", "body", "title", "subtitle", "label", "code", "muted"); e != nil {
			return e
		}
		if _, e := strField("text"); e != nil {
			return e
		}
		if v, ok := p["style"]; ok {
			if e := validateStyle(v); e != nil {
				return e
			}
		}
	case NodeViewport:
		if _, e := intField("height", 1); e != nil {
			return e
		}
		if e := boolField("wrap"); e != nil {
			return e
		}
		if e := boolField("follow_tail"); e != nil {
			return e
		}
		if e := boolField("scrollable"); e != nil {
			return e
		}
	case NodeTable:
		if e := validateEnum("variant", "normal", "compact", "dense"); e != nil {
			return e
		}
		if e := boolField("selectable"); e != nil {
			return e
		}
		if v, ok := p["columns"]; ok {
			var cols []map[string]json.RawMessage
			if json.Unmarshal(v, &cols) != nil {
				return bad("/props/columns", "columns must be array")
			}
			for i, c := range cols {
				for k := range c {
					if k != "title" && k != "width" {
						e := NewError("schema.unknown_property", fmt.Sprintf("unknown column property %q", k))
						e.Path = fmt.Sprintf("/props/columns/%d/%s", i, k)
						return e
					}
				}
				var title string
				if raw, ok := c["title"]; !ok || json.Unmarshal(raw, &title) != nil || title == "" {
					return bad(fmt.Sprintf("/props/columns/%d/title", i), "column title is required")
				}
				if raw, ok := c["width"]; ok {
					var width int
					if json.Unmarshal(raw, &width) != nil || width < 1 {
						return bad(fmt.Sprintf("/props/columns/%d/width", i), "column width must be >= 1")
					}
				}
			}
		}
		var rows [][]string
		if v, ok := p["rows"]; ok {
			if json.Unmarshal(v, &rows) != nil {
				return bad("/props/rows", "rows must be string matrix")
			}
		}
		if _, e := strField("action"); e != nil {
			return e
		}
		if v, ok := p["row_ids"]; ok {
			var ids []string
			if json.Unmarshal(v, &ids) != nil {
				return bad("/props/row_ids", "row_ids must be string array")
			}
			if len(ids) != len(rows) {
				return bad("/props/row_ids", "row_ids length must match rows length")
			}
			seen := make(map[string]struct{}, len(ids))
			for i, id := range ids {
				if id == "" {
					return bad(fmt.Sprintf("/props/row_ids/%d", i), "row_id must be non-empty")
				}
				if _, exists := seen[id]; exists {
					return bad(fmt.Sprintf("/props/row_ids/%d", i), "row_ids must be unique")
				}
				seen[id] = struct{}{}
			}
		}
	case NodeInput:
		if _, e := strField("placeholder"); e != nil {
			return e
		}
		if _, e := strField("value"); e != nil {
			return e
		}
		if _, e := strField("action"); e != nil {
			return e
		}
	case NodeActions:
		if e := validateEnum("variant", "inline", "toolbar", "list"); e != nil {
			return e
		}
		if v, ok := p["items"]; ok {
			var items []map[string]json.RawMessage
			if json.Unmarshal(v, &items) != nil {
				return bad("/props/items", "items must be array")
			}
			for i, it := range items {
				for k := range it {
					if k != "key" && k != "label" && k != "action" && k != "args" {
						e := NewError("schema.unknown_property", fmt.Sprintf("unknown action item property %q", k))
						e.Path = fmt.Sprintf("/props/items/%d/%s", i, k)
						return e
					}
				}
				var key, label, action string
				if json.Unmarshal(it["key"], &key) != nil || len([]rune(key)) != 1 || json.Unmarshal(it["label"], &label) != nil || label == "" || json.Unmarshal(it["action"], &action) != nil || action == "" {
					return bad(fmt.Sprintf("/props/items/%d", i), "key must be one rune and label/action required")
				}
				if raw, ok := it["args"]; ok {
					var args map[string]json.RawMessage
					if json.Unmarshal(raw, &args) != nil {
						return bad(fmt.Sprintf("/props/items/%d/args", i), "args must be object")
					}
				}
			}
		}
	case NodeProgress:
		if e := validateEnum("variant", "bar", "compact", "spinner"); e != nil {
			return e
		}
		if e := validateEnum("state", "normal", "loading", "success", "error"); e != nil {
			return e
		}
		if v, ok := p["value"]; ok {
			var n float64
			if json.Unmarshal(v, &n) != nil || n < 0 || n > 1 {
				return bad("/props/value", "value must be 0..1")
			}
		}
		if _, e := strField("label"); e != nil {
			return e
		}
	}
	return nil
}

func MergeNormalized(t NodeType, current map[string]json.RawMessage, raw json.RawMessage) (map[string]json.RawMessage, MutationPolicy, *Error) {
	patch, policy, err := NormalizeProps(t, raw, false)
	if err != nil {
		return nil, policy, err
	}
	merged := cloneProps(rawProps(current))
	for k, v := range patch {
		merged[k] = v
	}
	if err := validateNormalized(t, merged); err != nil {
		return nil, policy, err
	}
	return map[string]json.RawMessage(merged), policy, nil
}

func MarshalProps(p map[string]json.RawMessage) json.RawMessage { b, _ := json.Marshal(p); return b }
