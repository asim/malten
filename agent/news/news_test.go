package news

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadHeadlines(t *testing.T) {
	payload := `{"items":[
 {"category":"Bad","title":"Unsafe","url":"javascript:alert(1)"},
 {"category":"World","title":"One","url":"https://example.com/one"},
 {"category":"World","title":"Duplicate topic","url":"https://example.com/two"},
 {"category":"Tech","title":"Two","url":"https://example.com/three"},
 {"category":"Finance","title":"Three","url":"https://example.com/four"},
 {"category":"Other","title":"Four","url":"https://example.com/five"}]}`
	raw, _ := json.Marshal(map[string]any{"result": map[string]any{"content": []any{map[string]string{"type": "text", "text": payload}}}})
	text, err := readHeadlines(strings.NewReader(string(raw)))
	if err != nil || !strings.Contains(text, "World · One") || strings.Contains(text, "Unsafe") || strings.Contains(text, "Duplicate topic") || strings.Contains(text, "Other ·") {
		t.Fatalf("%s: %v", text, err)
	}
	if len([]rune(text)) > 1200 {
		t.Fatal("post too large")
	}
	for _, raw := range []string{`{"error":{"code":-1}}`, `{"result":{"isError":true}}`, `{"result":{"content":[]}}`} {
		if _, err := readHeadlines(strings.NewReader(raw)); err == nil {
			t.Fatal("accepted failed or empty response")
		}
	}
}
