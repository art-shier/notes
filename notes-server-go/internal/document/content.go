package document

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"math"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

var blocks = map[string]bool{"paragraph": true, "heading": true, "bulletList": true, "orderedList": true, "blockquote": true, "codeBlock": true, "image": true, "horizontalRule": true}
var children = map[string]map[string]bool{"doc": blocks, "paragraph": {"text": true, "hardBreak": true}, "heading": {"text": true, "hardBreak": true}, "bulletList": {"listItem": true}, "orderedList": {"listItem": true}, "listItem": blocks, "blockquote": blocks, "codeBlock": {"text": true}}
var allowedAttrs = map[string]map[string]bool{"heading": {"level": true}, "orderedList": {"start": true, "type": true}, "codeBlock": {"language": true}, "image": {"attachment_id": true, "alt": true, "title": true, "src": true, "width": true, "height": true}}

func invalid() error {
	return &Error{422, "invalid_content", "正文含不支持的节点、属性或链接。"}
}
func object(v any) (Doc, bool) { d, ok := v.(map[string]any); return d, ok }
func nodes(d Doc) []Doc {
	var out []Doc
	switch values := d["content"].(type) {
	case []any:
		for _, v := range values {
			if n, ok := object(v); ok {
				out = append(out, n)
			}
		}
	case []Doc:
		out = values
	}
	return out
}

// Children returns top-level nodes without embedding storage or HTTP concerns.
func Children(d Doc) []Doc { return nodes(d) }
func anyNodes(ns []Doc) []any {
	out := make([]any, len(ns))
	for i, n := range ns {
		out[i] = n
	}
	return out
}
func attrs(d Doc) Doc {
	a, _ := object(d["attrs"])
	if a == nil {
		return Doc{}
	}
	return a
}
func integer(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), int64(int(n)) == n
	case float64:
		if !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && n >= -2147483648 && n <= 2147483647 {
			return int(n), true
		}
	case json.Number:
		i, e := n.Int64()
		return int(i), e == nil
	}
	return 0, false
}
func serialize(d any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(d)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// The legacy limit counts Unicode characters in json.dumps(ensure_ascii=False),
// including the spaces after commas and colons in its default separators.
func contentSize(data []byte) int {
	size := utf8.RuneCount(data)
	inString := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			if c == '\\' {
				if i+5 < len(data) && (string(data[i:i+6]) == `\u2028` || string(data[i:i+6]) == `\u2029`) {
					size -= 5
					i += 5
				} else {
					i++
				}
			} else if c == '"' {
				inString = false
			}
		} else if c == '"' {
			inString = true
		} else if c == ',' || c == ':' {
			size++
		}
	}
	return size
}
func clone(v any) any {
	switch x := v.(type) {
	case map[string]any:
		o := Doc{}
		for k, val := range x {
			o[k] = clone(val)
		}
		return o
	case []any:
		o := make([]any, len(x))
		for i, val := range x {
			o[i] = clone(val)
		}
		return o
	case []Doc:
		o := make([]any, len(x))
		for i, val := range x {
			o[i] = clone(val)
		}
		return o
	}
	return v
}
func safeLink(s string) bool {
	u, e := url.Parse(s)
	if e != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

func Validate(d Doc, validateAttachments AttachmentValidator) (Doc, error) {
	data := serialize(d)
	if len(data) == 0 {
		return nil, invalid()
	}
	if contentSize(data) > 1<<20 {
		return nil, &Error{413, "content_size", "笔记正文超过 1 MiB。"}
	}
	if d["type"] != "doc" {
		return nil, &Error{422, "invalid_content", "正文必须是文档。"}
	}
	count := 0
	ids := map[string]bool{}
	var walk func(Doc, int) (Doc, error)
	walk = func(n Doc, depth int) (Doc, error) {
		count++
		if n == nil || depth > 32 || count > 10000 {
			return nil, invalid()
		}
		for k := range n {
			if k != "type" && k != "attrs" && k != "content" && k != "text" && k != "marks" {
				return nil, invalid()
			}
		}
		kind, ok := n["type"].(string)
		if !ok {
			return nil, invalid()
		}
		if _, ok := children[kind]; !ok && kind != "text" && kind != "hardBreak" && kind != "image" && kind != "horizontalRule" {
			return nil, invalid()
		}
		a := Doc{}
		if v := n["attrs"]; v != nil {
			var ok bool
			a, ok = object(v)
			if !ok {
				return nil, invalid()
			}
		}
		for k := range a {
			if !allowedAttrs[kind][k] {
				return nil, invalid()
			}
		}
		out := Doc{"type": kind}
		if kind == "text" {
			text, ok := n["text"].(string)
			if !ok || text == "" {
				return nil, invalid()
			}
			if _, ok := n["content"]; ok {
				return nil, invalid()
			}
			out["text"] = text
		} else if _, ok := n["text"]; ok {
			return nil, invalid()
		}
		switch kind {
		case "heading":
			v := a["level"]
			if _, ok := a["level"]; !ok {
				v = 2
			}
			level, ok := integer(v)
			if !ok || level < 1 || level > 6 {
				return nil, invalid()
			}
			out["attrs"] = Doc{"level": level}
		case "orderedList":
			v := a["start"]
			if _, ok := a["start"]; !ok {
				v = 1
			}
			start, ok := integer(v)
			if !ok || start < 1 || start > 100000 {
				return nil, invalid()
			}
			clean := Doc{"start": start}
			if style := a["type"]; style != nil {
				if style != "1" && style != "a" && style != "A" && style != "i" && style != "I" {
					return nil, invalid()
				}
				clean["type"] = style
			}
			out["attrs"] = clean
		case "codeBlock":
			v := a["language"]
			if v != nil {
				s, ok := v.(string)
				if !ok || utf8.RuneCountInString(s) > 40 {
					return nil, invalid()
				}
			}
			out["attrs"] = Doc{"language": v}
		case "image":
			raw, ok := a["attachment_id"].(string)
			if !ok {
				return nil, invalid()
			}
			id, e := uuid.Parse(raw)
			if e != nil {
				return nil, invalid()
			}
			aid := id.String()
			ids[aid] = true
			alt := ""
			if v := a["alt"]; v != nil {
				var ok bool
				alt, ok = v.(string)
				if !ok || utf8.RuneCountInString(alt) > 500 {
					return nil, invalid()
				}
			}
			title := a["title"]
			if title != nil {
				s, ok := title.(string)
				if !ok || utf8.RuneCountInString(s) > 500 {
					return nil, invalid()
				}
			}
			clean := Doc{"attachment_id": aid, "alt": alt, "title": title}
			for _, k := range []string{"width", "height"} {
				if v := a[k]; v != nil {
					size, ok := integer(v)
					if !ok || size < 1 || size > 10000 {
						return nil, invalid()
					}
					clean[k] = size
				}
			}
			out["attrs"] = clean
		}
		if raw, exists := n["marks"]; exists {
			if kind != "text" {
				return nil, invalid()
			}
			var list []any
			switch v := raw.(type) {
			case []any:
				list = v
			case []Doc:
				list = anyNodes(v)
			default:
				return nil, invalid()
			}
			marks := make([]any, 0, len(list))
			for _, v := range list {
				m, ok := object(v)
				if !ok {
					return nil, invalid()
				}
				for k := range m {
					if k != "type" && k != "attrs" {
						return nil, invalid()
					}
				}
				mk, ok := m["type"].(string)
				if !ok {
					return nil, invalid()
				}
				clean := Doc{"type": mk}
				switch mk {
				case "link":
					ma := attrs(m)
					href, ok := ma["href"].(string)
					if !ok || utf8.RuneCountInString(href) > 2000 || !safeLink(href) {
						return nil, invalid()
					}
					clean["attrs"] = Doc{"href": href, "target": "_blank", "rel": "noopener noreferrer nofollow"}
				case "bold", "italic", "strike", "code", "underline":
					if v := m["attrs"]; v != nil {
						ma, ok := object(v)
						if !ok || len(ma) > 0 {
							return nil, invalid()
						}
					}
				default:
					return nil, invalid()
				}
				marks = append(marks, clean)
			}
			out["marks"] = marks
		}
		if raw, exists := n["content"]; exists {
			allowed, ok := children[kind]
			if !ok {
				return nil, invalid()
			}
			var list []any
			switch v := raw.(type) {
			case []any:
				list = v
			case []Doc:
				list = anyNodes(v)
			default:
				return nil, invalid()
			}
			clean := []any{}
			for _, v := range list {
				child, ok := object(v)
				if !ok {
					return nil, invalid()
				}
				ck, ok := child["type"].(string)
				if !ok || !allowed[ck] {
					return nil, invalid()
				}
				c, e := walk(child, depth+1)
				if e != nil {
					return nil, e
				}
				clean = append(clean, c)
			}
			if len(clean) > 0 {
				out["content"] = clean
			}
		}
		if kind == "bulletList" || kind == "orderedList" || kind == "listItem" || kind == "blockquote" {
			cs := nodes(out)
			if len(cs) == 0 || kind == "listItem" && cs[0]["type"] != "paragraph" {
				return nil, invalid()
			}
		}
		return out, nil
	}
	clean, e := walk(d, 0)
	if e != nil {
		return nil, e
	}
	if len(ids) > 0 && validateAttachments != nil {
		list := make([]string, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		sort.Strings(list)
		if e := validateAttachments(list); e != nil {
			return nil, e
		}
	}
	return clean, nil
}
func PlainText(d Doc) string {
	if d["type"] == "text" {
		s, _ := d["text"].(string)
		return s
	}
	var b strings.Builder
	for _, c := range nodes(d) {
		b.WriteString(PlainText(c))
	}
	kind, _ := d["type"].(string)
	if blocks[kind] || kind == "listItem" || kind == "hardBreak" {
		b.WriteByte('\n')
	}
	return b.String()
}
func Hydrate(d Doc) Doc {
	out := clone(d).(Doc)
	var walk func(Doc)
	walk = func(n Doc) {
		if n["type"] == "image" {
			a := attrs(n)
			id, _ := a["attachment_id"].(string)
			a["src"] = "/api/v1/attachments/" + id
			n["attrs"] = a
		}
		for _, c := range nodes(n) {
			walk(c)
		}
	}
	walk(out)
	return out
}
func Thumbnail(d Doc) string {
	if d["type"] == "image" {
		id, _ := attrs(d)["attachment_id"].(string)
		return "/api/v1/attachments/" + id
	}
	for _, c := range nodes(d) {
		if s := Thumbnail(c); s != "" {
			return s
		}
	}
	return ""
}
func ImageIDs(d Doc) []string {
	set := map[string]bool{}
	var walk func(Doc)
	walk = func(n Doc) {
		if n["type"] == "image" {
			id, _ := attrs(n)["attachment_id"].(string)
			set[id] = true
		}
		for _, c := range nodes(n) {
			walk(c)
		}
	}
	walk(d)
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
