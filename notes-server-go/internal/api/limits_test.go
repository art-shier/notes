package api

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http/httptest"
	"shiji/internal/config"
	"strings"
	"testing"
	"time"
)

func TestLongUnicodeJSONRemainsSavable(t *testing.T) {
	f := newNotesFixture(t)
	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": strings.Repeat("中", 710000)}}}}}
	b := map[string]any{"folder_id": f.folder, "content_json": doc}
	raw, _ := json.Marshal(b)
	if len(raw) <= 2<<20 {
		t.Fatal("fixture too small")
	}
	f.must("POST", "/api/v1/notes", b, 201)
}

type blockedUploadReader struct {
	entered chan struct{}
	release <-chan struct{}
	read    bool
}

func (b *blockedUploadReader) Read(p []byte) (int, error) {
	if !b.read {
		b.read = true
		b.entered <- struct{}{}
		<-b.release
	}
	return 0, io.EOF
}
func TestConcurrentUploadWorkBoundedBeforeRead(t *testing.T) {
	a := New(nil, config.Settings{ExportConcurrency: 2, UploadLimit: 1024})
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan error, 2)
	invoke := func(reader io.Reader) error {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/api/v1/attachments", reader)
		c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=example")
		return a.upload(c, Principal{})
	}
	for i := 0; i < 2; i++ {
		go func() { done <- invoke(&blockedUploadReader{entered: entered, release: release}) }()
	}
	defer close(release)
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("upload did not reach reader")
		}
	}
	err := invoke(bytes.NewReader(nil))
	if e, ok := err.(*Error); !ok || e.Status != 429 || e.Code != "upload_busy" {
		t.Fatal("third upload entered processing", err)
	}
	release <- struct{}{}
	release <- struct{}{}
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("workers not released")
		}
	}
	err = invoke(bytes.NewReader(nil))
	if e, ok := err.(*Error); !ok || e.Status == 429 {
		t.Fatal("failed upload leaked slot", err)
	}
}
