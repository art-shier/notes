// Package document implements the canonical, bounded Tiptap document grammar.
package document

type Doc = map[string]any
type AttachmentValidator func([]string) error
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Operation struct {
	Op      string  `json:"op"`
	BlockID *string `json:"block_id"`
	AfterID *string `json:"after_id"`
	Content Doc     `json:"content"`
}
