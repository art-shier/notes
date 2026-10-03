package document

import (
	"html"
	"strconv"
	"strings"
)

const Style = "body{max-width:760px;margin:40px auto;padding:0 24px;font:16px/1.8 system-ui,sans-serif;color:#242428;background:#fff;overflow-wrap:anywhere}h1{line-height:1.4}img{max-width:100%;height:auto;display:block;margin:24px 0}pre{white-space:pre-wrap;background:#f5f5f7;padding:16px;border-radius:8px}blockquote{border-left:3px solid #ddd;padding-left:16px;margin-left:0;color:#666}a{color:#3269d4}.meta{font-size:13px;color:#686870}li{margin:6px 0}code{background:#f5f5f7}hr{border:0;border-top:1px solid #ddd}"

func Page(title, body string) string {
	return `<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self' file:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>` + html.EscapeString(title) + `</title><style>` + Style + `</style></head><body>` + body + `</body></html>`
}

// HTML renders an offline document fragment using only caller-provided image paths.
func HTML(d Doc, images map[string]string) (string, error) {
	kind, _ := d["type"].(string)
	a := attrs(d)
	switch kind {
	case "text":
		s, _ := d["text"].(string)
		out := html.EscapeString(s)
		for _, m := range markNodes(d) {
			mk, _ := m["type"].(string)
			tag := map[string]string{"bold": "strong", "italic": "em", "strike": "s", "underline": "u", "code": "code"}[mk]
			if tag != "" {
				out = "<" + tag + ">" + out + "</" + tag + ">"
			} else if mk == "link" {
				href, _ := attrs(m)["href"].(string)
				if safeLink(href) {
					out = `<a href="` + html.EscapeString(href) + `" target="_blank" rel="noopener noreferrer">` + out + `</a>`
				}
			}
		}
		return out, nil
	case "hardBreak":
		return "<br>", nil
	case "horizontalRule":
		return "<hr>", nil
	case "image":
		id, _ := a["attachment_id"].(string)
		path, ok := images[id]
		if !ok {
			return "", &Error{409, "attachment_missing", "笔记引用的图片不可用，导出未完成。"}
		}
		alt, _ := a["alt"].(string)
		out := `<img src="../` + html.EscapeString(path) + `" alt="` + html.EscapeString(alt) + `"`
		if title, _ := a["title"].(string); title != "" {
			out += ` title="` + html.EscapeString(title) + `"`
		}
		for _, key := range []string{"width", "height"} {
			if val, ok := integer(a[key]); ok {
				out += " " + key + `="` + strconv.Itoa(val) + `"`
			}
		}
		return out + ">", nil
	}
	var b strings.Builder
	for _, c := range nodes(d) {
		s, e := HTML(c, images)
		if e != nil {
			return "", e
		}
		b.WriteString(s)
	}
	inner := b.String()
	if kind == "doc" {
		return inner, nil
	}
	tag := map[string]string{"paragraph": "p", "bulletList": "ul", "orderedList": "ol", "listItem": "li", "blockquote": "blockquote", "codeBlock": "pre"}[kind]
	if kind == "heading" {
		level, ok := integer(a["level"])
		if !ok {
			level = 2
		}
		tag = "h" + strconv.Itoa(level)
	}
	if tag == "" {
		return "", &Error{409, "invalid_archive_content", "正文包含无法导出的节点。"}
	}
	if kind == "codeBlock" {
		inner = "<code>" + inner + "</code>"
	}
	attributes := ""
	if kind == "orderedList" {
		start, ok := integer(a["start"])
		if !ok {
			start = 1
		}
		attributes = ` start="` + strconv.Itoa(start) + `"`
		if style, _ := a["type"].(string); style != "" {
			attributes += ` type="` + html.EscapeString(style) + `"`
		}
	}
	return "<" + tag + attributes + ">" + inner + "</" + tag + ">", nil
}
