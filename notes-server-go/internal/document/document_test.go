package document

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func readDoc(t *testing.T, s string) Doc {
	t.Helper()
	var d Doc
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatal(err)
	}
	return d
}
func checkCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := err.(*Error)
	if !ok || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}
func TestValidateStrictGrammar(t *testing.T) {
	for _, s := range []string{`{"type":"doc","extra":1}`, `{"type":"doc","content":[{"type":"text","text":"inline"}]}`, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":""}]}]}`, `{"type":"doc","content":[{"type":"heading","attrs":{"level":true}}]}`, `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"heading"}]}]}]}`, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:evil()"}}]}]}]}`} {
		_, err := Validate(readDoc(t, s), nil)
		checkCode(t, err, "invalid_content")
	}
	d, err := Validate(readDoc(t, `{"type":"doc","content":[{"type":"heading"},{"type":"codeBlock"},{"type":"orderedList","content":[{"type":"listItem","content":[{"type":"paragraph"}]}]}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if d["content"].([]any)[0].(Doc)["attrs"].(Doc)["level"] != 2 {
		t.Fatal(d)
	}
}
func TestImageOwnershipAndHydration(t *testing.T) {
	const aid = "12345678-1234-1234-1234-123456789abc"
	d := readDoc(t, `{"type":"doc","content":[{"type":"image","attrs":{"attachment_id":"12345678-1234-1234-1234-123456789abc","src":"https://evil","width":null,"height":42}}]}`)
	_, err := Validate(d, func(ids []string) error { return &Error{Status: 404, Code: "not_found"} })
	checkCode(t, err, "not_found")
	clean, err := Validate(d, func(ids []string) error {
		if !reflect.DeepEqual(ids, []string{aid}) {
			t.Fatal(ids)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	image := clean["content"].([]any)[0].(Doc)
	if _, ok := image["attrs"].(Doc)["src"]; ok {
		t.Fatal(clean)
	}
	if Thumbnail(clean) != "/api/v1/attachments/"+aid {
		t.Fatal(Thumbnail(clean))
	}
	hydrated := Hydrate(clean)
	if hydrated["content"].([]any)[0].(Doc)["attrs"].(Doc)["src"] != "/api/v1/attachments/"+aid {
		t.Fatal(hydrated)
	}
	if _, ok := image["attrs"].(Doc)["src"]; ok {
		t.Fatal("mutated input")
	}
}
func TestContentLimits(t *testing.T) {
	d := Doc{"type": "doc", "content": []any{Doc{"type": "paragraph", "content": []any{Doc{"type": "text", "text": strings.Repeat("x", 1<<20)}}}}}
	_, err := Validate(d, nil)
	checkCode(t, err, "content_size")
	var node Doc = Doc{"type": "paragraph"}
	for i := 0; i < 33; i++ {
		node = Doc{"type": "blockquote", "content": []any{node}}
	}
	_, err = Validate(Doc{"type": "doc", "content": []any{node}}, nil)
	checkCode(t, err, "invalid_content")
	nodes := make([]any, 10000)
	for i := range nodes {
		nodes[i] = Doc{"type": "paragraph"}
	}
	_, err = Validate(Doc{"type": "doc", "content": nodes}, nil)
	checkCode(t, err, "invalid_content")
}

func TestContentSizeIncludesJSONSpacing(t *testing.T) {
	// The existing contract measures json.dumps with its default separators:
	// this document adds 94 characters around its text value.
	d := Doc{"type": "doc", "content": []any{Doc{"type": "paragraph", "content": []any{Doc{"type": "text", "text": strings.Repeat("x", (1<<20)-90)}}}}}
	_, err := Validate(d, nil)
	checkCode(t, err, "content_size")
}
func TestMarkdownRoundtripAndWarnings(t *testing.T) {
	md := "# 标题\n\n正文 **加粗** 和 *斜体*，以及 [链接](https://example.com)。\n\n> 引用\n\n1. 第一项\n2. 第二项\n\n```python\nprint(\"hello\")\n```"
	d, w, err := FromMarkdown(md)
	if err != nil || len(w) != 0 {
		t.Fatalf("%v %v", w, err)
	}
	d, err = Validate(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, safe := ExportMarkdown(d)
	if !safe || !strings.Contains(out, "**加粗**") {
		t.Fatalf("safe=%v\n%s\n%#v", safe, out, d)
	}
	if !strings.Contains(PlainText(d), "标题\n正文 加粗") {
		t.Fatal(PlainText(d))
	}
	_, w, err = FromMarkdown("| A | B |\n| --- | --- |\n| 1 | 2 |\n\n- [x] done\n\n<div>text</div>")
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, v := range w {
		codes[v.Code] = true
	}
	for _, c := range []string{"table_as_text", "task_as_text", "html_as_text"} {
		if !codes[c] {
			t.Fatal(w)
		}
	}
	d = readDoc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"下划线","marks":[{"type":"underline"}]}]}]}`)
	_, safe = ExportMarkdown(d)
	if safe {
		t.Fatal("underline roundtrip incorrectly safe")
	}
}
func TestMarkdownImagesAndLeadingLists(t *testing.T) {
	_, _, err := FromMarkdown("![外链](https://example.com/a.png)")
	checkCode(t, err, "external_image")
	d, w, err := FromMarkdown("- # 列表里的标题\n- ![图片](attachment://12345678-1234-1234-1234-123456789abc)\n- > 引用\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 1 || w[0].Code != "list_leading_paragraph" {
		t.Fatal(w)
	}
	d, err = Validate(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, safe := ExportMarkdown(d)
	if !safe {
		t.Fatalf("not safe: %s %#v", out, d)
	}
}
func TestBlockAlignmentAndAtomicOperations(t *testing.T) {
	a := Doc{"type": "paragraph", "content": []any{Doc{"type": "text", "text": "A"}}}
	b := Doc{"type": "paragraph", "content": []any{Doc{"type": "text", "text": "B"}}}
	x := Doc{"type": "heading", "attrs": Doc{"level": 2}}
	ids := AlignIDs([]Doc{a, b}, []string{"a", "b"}, []Doc{x, a, b})
	if !reflect.DeepEqual(ids[1:], []string{"a", "b"}) {
		t.Fatal(ids)
	}
	old := Doc{"type": "doc", "content": []any{a, b}}
	target := "a"
	missing := "missing"
	_, _, err := ApplyOperations(old, []string{"a", "b"}, []Operation{{Op: "delete", BlockID: &target}, {Op: "delete", BlockID: &missing}}, nil)
	checkCode(t, err, "unknown_block")
	if len(old["content"].([]any)) != 2 {
		t.Fatal("mutated input")
	}
	d, ids, err := ApplyOperations(old, []string{"a", "b"}, []Operation{{Op: "replace", BlockID: &target, Content: x}, {Op: "insert", AfterID: &target, Content: a}}, nil)
	if err != nil || len(ids) != 3 || ids[0] != "a" || ids[2] != "b" || len(d["content"].([]any)) != 3 {
		t.Fatalf("%v %v %v", d, ids, err)
	}
}
func TestHTMLSafeOfflineImages(t *testing.T) {
	d := readDoc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<script>evil</script>","marks":[{"type":"link","attrs":{"href":"https://example.com/?q=\"bad\""}}]}]},{"type":"image","attrs":{"attachment_id":"12345678-1234-1234-1234-123456789abc","alt":"\"><script>evil</script>","width":10}}]}`)
	out, err := HTML(d, map[string]string{"12345678-1234-1234-1234-123456789abc": "attachments/photo.png"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<script>") || !strings.Contains(out, `src="../attachments/photo.png"`) || !strings.Contains(out, `width="10"`) {
		t.Fatal(out)
	}
	_, err = HTML(d, nil)
	checkCode(t, err, "attachment_missing")
}

func TestMarkdownInlineSemantics(t *testing.T) {
	d, _, err := FromMarkdown("single ~plain~ and ~~strike~~\n\nA  \nB\n\n` a ` and ``a`b``\n\n&lt;safe&gt; \\*literal\\*")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(PlainText(d), "single ~plain~ and strike") {
		t.Fatal(PlainText(d))
	}
	out, safe := ExportMarkdown(d)
	if !safe {
		t.Fatalf("unsafe inline: %s %#v", out, d)
	}
	_, _, err = FromMarkdown(strings.Repeat("x", (1<<20)+1))
	checkCode(t, err, "content_size")
}

func TestCanonicalMergesAdjacentTextAndNormalizesMarks(t *testing.T) {
	original := readDoc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a","marks":[{"type":"bold"},{"type":"italic"}]},{"type":"text","text":"b","marks":[{"type":"italic"},{"type":"bold"}]}]}]}`)
	want := readDoc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"ab","marks":[{"type":"bold"},{"type":"italic"}]}]}]}`)
	if !bytesEqual(Canonical(original), want) {
		t.Fatal(Canonical(original))
	}
	if !bytesEqual(Canonical(Doc{"type": "doc", "content": []any{Doc{"type": "paragraph"}}}), Doc{"type": "doc"}) {
		t.Fatal("empty paragraph mismatch")
	}
}

func TestAtomicOperationsRejectInvalidContent(t *testing.T) {
	input := Doc{"type": "doc", "content": []any{Doc{"type": "paragraph"}}}
	id := "a"
	for _, ops := range [][]Operation{nil, {{Op: "insert", BlockID: &id, Content: Doc{"type": "paragraph"}}}, {{Op: "insert", Content: Doc{"type": "script"}}}, {{Op: "delete", BlockID: &id, Content: Doc{"type": "paragraph"}}}, {{Op: "replace", BlockID: &id}}, {{Op: "oops"}}} {
		_, _, err := ApplyOperations(input, []string{id}, ops, nil)
		if err == nil {
			t.Fatalf("accepted %v", ops)
		}
	}
	output, ids, err := ApplyOperations(input, []string{id}, []Operation{{Op: "delete", BlockID: &id}}, nil)
	if err != nil || len(ids) != 0 || output["type"] != "doc" {
		t.Fatalf("%v %v %v", output, ids, err)
	}
}
