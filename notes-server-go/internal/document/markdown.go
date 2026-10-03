package document

import (
	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var tablePattern = regexp.MustCompile(`(?m)^\s*\|?.*\|.*\n\s*\|?\s*:?-{3,}`)
var taskPattern = regexp.MustCompile(`(?m)^\s*[-*+] \[[ xX]\]`)
var htmlPattern = regexp.MustCompile(`</?[A-Za-z][\w:-]*(?:\s[^<>]*|/?)>`)
var backticks = regexp.MustCompile("`+")
var escapePattern = regexp.MustCompile("([\\\\`*{}_\\[\\]()#+.!>|~\\-])")

// CommonMark's optional strikethrough uses pairs of tildes; a single tilde
// remains literal, as it did in the Python markdown-it conversion.
type doubleStrikeParser struct{}

func (doubleStrikeParser) Trigger() []byte { return []byte{'~'} }
func (doubleStrikeParser) Parse(parent ast.Node, reader text.Reader, context parser.Context) ast.Node {
	line, _ := reader.PeekLine()
	if len(line) < 2 || line[0] != '~' || line[1] != '~' {
		return nil
	}
	return extension.NewStrikethroughParser().Parse(parent, reader, context)
}

func markdownParser() goldmark.Markdown {
	bp := []util.PrioritizedValue{}
	for _, v := range parser.DefaultBlockParsers() {
		if v.Priority != 900 {
			bp = append(bp, v)
		}
	}
	ip := []util.PrioritizedValue{}
	for _, v := range parser.DefaultInlineParsers() {
		if v.Priority != 400 {
			ip = append(ip, v)
		}
	}

	ip = append(ip, util.Prioritized(doubleStrikeParser{}, 500))
	p := parser.NewParser(parser.WithBlockParsers(bp...), parser.WithInlineParsers(ip...), parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...))
	return goldmark.New(goldmark.WithParser(p))
}
func resolved(v []byte) string {
	return string(util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(v))))
}
func textNode(s string, marks []Doc) Doc {
	n := Doc{"type": "text", "text": s}
	if len(marks) > 0 {
		n["marks"] = clone(anyNodes(marks))
	}
	return n
}
func FromMarkdown(value string) (Doc, []Warning, error) {
	if len(value) > 1<<20 {
		return nil, nil, &Error{413, "content_size", "Markdown 超过 1 MiB。"}
	}
	warnings := []Warning{}
	if tablePattern.MatchString(value) {
		warnings = append(warnings, Warning{"table_as_text", "本阶段 Markdown 表格作为普通文字保留。"})
	}
	if taskPattern.MatchString(value) {
		warnings = append(warnings, Warning{"task_as_text", "待办符号作为列表文字保留。"})
	}
	if htmlPattern.MatchString(value) {
		warnings = append(warnings, Warning{"html_as_text", "HTML 标记作为文字保留，不执行 HTML。"})
	}
	source := []byte(value)
	tree := markdownParser().Parser().Parse(text.NewReader(source))
	count := 0
	var inlineChildren func(ast.Node, []Doc, int) ([]Doc, error)
	inlineChildren = func(parent ast.Node, marks []Doc, depth int) ([]Doc, error) {
		if depth > 32 {
			return nil, invalid()
		}
		out := []Doc{}
		for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
			count++
			if count > 10000 {
				return nil, invalid()
			}
			switch n := c.(type) {
			case *ast.Text:
				s := string(n.Value(source))
				if !n.IsRaw() {
					s = resolved(n.Value(source))
				}
				if s != "" {
					out = append(out, textNode(s, marks))
				}
				if n.HardLineBreak() {
					out = append(out, Doc{"type": "hardBreak"})
				} else if n.SoftLineBreak() {
					out = append(out, textNode("\n", marks))
				}
			case *ast.String:
				s := string(n.Value)
				if !n.IsRaw() {
					s = resolved(n.Value)
				}
				if s != "" {
					out = append(out, textNode(s, marks))
				}
			case *ast.CodeSpan:
				var b strings.Builder
				for t := n.FirstChild(); t != nil; t = t.NextSibling() {
					tn := t.(*ast.Text)
					s := string(tn.Value(source))
					if strings.HasSuffix(s, "\n") {
						s = strings.TrimSuffix(s, "\n") + " "
					}
					b.WriteString(s)
				}
				if b.Len() > 0 {
					active := append(append([]Doc{}, marks...), Doc{"type": "code"})
					out = append(out, textNode(b.String(), active))
				}
			case *ast.Emphasis:
				kind := "italic"
				if n.Level == 2 {
					kind = "bold"
				}
				active := append(append([]Doc{}, marks...), Doc{"type": kind})
				ns, e := inlineChildren(n, active, depth+1)
				if e != nil {
					return nil, e
				}
				out = append(out, ns...)
			case *east.Strikethrough:
				active := append(append([]Doc{}, marks...), Doc{"type": "strike"})
				ns, e := inlineChildren(n, active, depth+1)
				if e != nil {
					return nil, e
				}
				out = append(out, ns...)
			case *ast.Link:
				href := resolved(n.Destination)
				active := append(append([]Doc{}, marks...), Doc{"type": "link", "attrs": Doc{"href": href}})
				ns, e := inlineChildren(n, active, depth+1)
				if e != nil {
					return nil, e
				}
				out = append(out, ns...)
			case *ast.AutoLink:
				href := string(n.URL(source))
				if n.AutoLinkType == ast.AutoLinkEmail {
					href = "mailto:" + href
				}
				active := append(append([]Doc{}, marks...), Doc{"type": "link", "attrs": Doc{"href": href}})
				out = append(out, textNode(string(n.Label(source)), active))
			case *ast.Image:
				dest := resolved(n.Destination)
				if !strings.HasPrefix(dest, "attachment://") {
					return nil, &Error{422, "external_image", "图片需要先上传，并使用 attachment://UUID 引用。"}
				}
				id, e := uuid.Parse(strings.TrimPrefix(dest, "attachment://"))
				if e != nil {
					return nil, &Error{422, "external_image", "图片需要先上传，并使用 attachment://UUID 引用。"}
				}
				ns, e := inlineChildren(n, nil, depth+1)
				if e != nil {
					return nil, e
				}
				var alt strings.Builder
				for _, v := range ns {
					if s, ok := v["text"].(string); ok {
						alt.WriteString(s)
					}
				}
				var title any
				if n.Title != nil {
					title = resolved(n.Title)
				}
				out = append(out, Doc{"type": "image", "attrs": Doc{"attachment_id": id.String(), "alt": alt.String(), "title": title}})
			default:
				return nil, &Error{422, "unsupported_markdown", "Markdown 包含不支持的语法，请使用 JSON。"}
			}
		}
		return out, nil
	}
	normalized := false
	var convert func(ast.Node, int) ([]Doc, error)
	convert = func(n ast.Node, depth int) ([]Doc, error) {
		count++
		if depth > 32 || count > 10000 {
			return nil, invalid()
		}
		kind := ""
		a := Doc{}
		switch v := n.(type) {
		case *ast.Document:
			kind = "doc"
		case *ast.Paragraph, *ast.TextBlock:
			kind = "paragraph"
		case *ast.Heading:
			kind = "heading"
			a["level"] = v.Level
		case *ast.List:
			kind = "bulletList"
			if v.IsOrdered() {
				kind = "orderedList"
				a["start"] = v.Start
			}
		case *ast.ListItem:
			kind = "listItem"
		case *ast.Blockquote:
			kind = "blockquote"
		case *ast.ThematicBreak:
			return []Doc{{"type": "horizontalRule"}}, nil
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			var language any
			if v, ok := n.(*ast.FencedCodeBlock); ok {
				if fields := strings.Fields(resolved(v.Language(source))); len(fields) > 0 {
					language = fields[0]
				}
			}
			raw := strings.TrimSuffix(string(n.Lines().Value(source)), "\n")
			node := Doc{"type": "codeBlock", "attrs": Doc{"language": language}, "content": []any{}}
			if raw != "" {
				node["content"] = []any{textNode(raw, nil)}
			}
			return []Doc{node}, nil
		default:
			return nil, &Error{422, "unsupported_markdown", "Markdown 包含不支持的块，请使用 JSON。"}
		}
		node := Doc{"type": kind, "content": []any{}}
		if len(a) > 0 {
			node["attrs"] = a
		}
		if kind == "paragraph" || kind == "heading" {
			ns, e := inlineChildren(n, nil, depth+1)
			if e != nil {
				return nil, e
			}
			out := []Doc{}
			buffer := []Doc{}
			flush := func() {
				piece := clone(node).(Doc)
				piece["content"] = anyNodes(buffer)
				out = append(out, piece)
				buffer = nil
			}
			for _, v := range ns {
				if v["type"] == "image" {
					if len(buffer) > 0 {
						flush()
					}
					out = append(out, v)
				} else {
					buffer = append(buffer, v)
				}
			}
			if len(buffer) > 0 || len(out) == 0 {
				flush()
			}
			return out, nil
		}
		cs := []Doc{}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			ns, e := convert(c, depth+1)
			if e != nil {
				return nil, e
			}
			cs = append(cs, ns...)
		}
		if kind == "listItem" && (len(cs) == 0 || cs[0]["type"] != "paragraph") {
			cs = append([]Doc{{"type": "paragraph"}}, cs...)
			normalized = true
		}
		node["content"] = anyNodes(cs)
		return []Doc{node}, nil
	}
	docs, e := convert(tree, 0)
	if e != nil {
		return nil, nil, e
	}
	root := docs[0]
	if len(nodes(root)) == 0 {
		root["content"] = []any{Doc{"type": "paragraph"}}
	}
	if normalized {
		warnings = append(warnings, Warning{"list_leading_paragraph", "为兼容网页编辑器，列表项开头补入空段落，原标题、图片等内容保留。"})
	}
	return root, warnings, nil
}
func Escape(s string) string { return escapePattern.ReplaceAllString(s, `\$1`) }
func markNodes(d Doc) []Doc {
	raw := d["marks"]
	switch v := raw.(type) {
	case []Doc:
		return v
	case []any:
		out := []Doc{}
		for _, m := range v {
			if n, ok := object(m); ok {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}
func inlineMarkdown(d Doc) string {
	if d["type"] == "hardBreak" {
		return "  \n"
	}
	raw, _ := d["text"].(string)
	value := Escape(raw)
	for _, m := range markNodes(d) {
		switch m["type"] {
		case "code":
			width := 1
			for _, run := range backticks.FindAllString(raw, -1) {
				if len(run) >= width {
					width = len(run) + 1
				}
			}
			fence := strings.Repeat("`", width)
			pad := ""
			if strings.HasPrefix(raw, "`") || strings.HasPrefix(raw, " ") || strings.HasSuffix(raw, "`") || strings.HasSuffix(raw, " ") {
				pad = " "
			}
			value = fence + pad + raw + pad + fence
		case "bold":
			value = "**" + value + "**"
		case "italic":
			value = "*" + value + "*"
		case "strike":
			value = "~~" + value + "~~"
		case "link":
			href, _ := attrs(m)["href"].(string)
			value = "[" + value + "](<" + href + ">)"
		}
	}
	return value
}
func ToMarkdown(d Doc) string {
	kind, _ := d["type"].(string)
	cs := nodes(d)
	a := attrs(d)
	join := func(ns []Doc) string {
		ss := []string{}
		for _, c := range ns {
			ss = append(ss, ToMarkdown(c))
		}
		return strings.Join(ss, "\n\n")
	}
	switch kind {
	case "paragraph", "heading":
		var b strings.Builder
		if kind == "heading" {
			level, ok := integer(a["level"])
			if !ok {
				level = 2
			}
			b.WriteString(strings.Repeat("#", level) + " ")
		}
		for _, c := range cs {
			b.WriteString(inlineMarkdown(c))
		}
		return b.String()
	case "doc":
		return join(cs)
	case "blockquote":
		lines := strings.Split(join(cs), "\n")
		for i := range lines {
			lines[i] = "> " + lines[i]
		}
		return strings.Join(lines, "\n")
	case "bulletList", "orderedList":
		start, ok := integer(a["start"])
		if !ok {
			start = 1
		}
		lines := []string{}
		for i, c := range cs {
			marker := "- "
			if kind == "orderedList" {
				marker = strconv.Itoa(start+i) + ". "
			}
			parts := strings.Split(ToMarkdown(c), "\n")
			value := marker + parts[0]
			for _, line := range parts[1:] {
				value += "\n" + strings.Repeat(" ", len(marker)) + line
			}
			lines = append(lines, value)
		}
		return strings.Join(lines, "\n")
	case "listItem":
		if len(cs) > 1 && cs[0]["type"] == "paragraph" && len(nodes(cs[0])) == 0 {
			cs = cs[1:]
		}
		return join(cs)
	case "codeBlock":
		var b strings.Builder
		for _, c := range cs {
			s, _ := c["text"].(string)
			b.WriteString(s)
		}
		raw := b.String()
		width := 3
		for _, run := range backticks.FindAllString(raw, -1) {
			if len(run) >= width {
				width = len(run) + 1
			}
		}
		fence := strings.Repeat("`", width)
		language, _ := a["language"].(string)
		return fence + language + "\n" + raw + "\n" + fence
	case "image":
		alt, _ := a["alt"].(string)
		aid, _ := a["attachment_id"].(string)
		title, _ := a["title"].(string)
		suffix := ""
		if title != "" {
			suffix = " \"" + strings.ReplaceAll(strings.ReplaceAll(title, "\\", "\\\\"), "\"", "\\\"") + "\""
		}
		return "![" + Escape(alt) + "](attachment://" + aid + suffix + ")"
	case "horizontalRule":
		return "---"
	}
	return ""
}
func Canonical(d Doc) Doc {
	out := Doc{"type": d["type"]}
	a := Doc{}
	for k, v := range attrs(d) {
		if v != nil && k != "src" {
			a[k] = v
		}
	}
	if d["type"] == "image" {
		if _, ok := a["alt"]; !ok {
			a["alt"] = ""
		}
	}
	if d["type"] == "orderedList" {
		if _, ok := a["start"]; !ok {
			a["start"] = 1
		}
	}
	if len(a) > 0 {
		out["attrs"] = a
	}
	if v, ok := d["text"]; ok {
		out["text"] = v
	}
	marks := []Doc{}
	for _, m := range markNodes(d) {
		v := Doc{"type": m["type"]}
		if m["type"] == "link" {
			v["attrs"] = Doc{"href": attrs(m)["href"]}
		}
		marks = append(marks, v)
	}
	if len(marks) > 0 {
		sort.SliceStable(marks, func(i, j int) bool { return marks[i]["type"].(string) < marks[j]["type"].(string) })
		out["marks"] = anyNodes(marks)
	}
	cs := []Doc{}
	for _, c := range nodes(d) {
		v := Canonical(c)
		if len(cs) > 0 && v["type"] == "text" && cs[len(cs)-1]["type"] == "text" && reflect.DeepEqual(cs[len(cs)-1]["marks"], v["marks"]) {
			cs[len(cs)-1]["text"] = cs[len(cs)-1]["text"].(string) + v["text"].(string)
		} else {
			cs = append(cs, v)
		}
	}
	if d["type"] == "doc" && len(cs) == 1 && reflect.DeepEqual(cs[0], Doc{"type": "paragraph"}) {
		cs = nil
	}
	if len(cs) > 0 {
		out["content"] = anyNodes(cs)
	}
	return out
}
func ExportMarkdown(d Doc) (value string, safe bool) {
	defer func() {
		if recover() != nil {
			safe = false
		}
	}()
	value = ToMarkdown(d)
	parsed, _, err := FromMarkdown(value)
	if err != nil {
		return value, false
	}
	return value, bytesEqual(Canonical(d), Canonical(parsed))
}
func bytesEqual(a, b Doc) bool { return string(serialize(a)) == string(serialize(b)) }
