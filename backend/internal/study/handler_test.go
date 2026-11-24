package study

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestParseHTTP(t *testing.T) {
	r := chi.NewRouter()
	New(nil, nil).Routes(r)
	text := "[тип: теорема]\n$||X||$\n---\n**Answer**"
	body, _ := json.Marshal(map[string]string{"text": text})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/parse", strings.NewReader(string(body))))
	var got ParseResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 || len(got.Cards) != 1 || got.Cards[0].Kind != "theorem" {
		t.Fatalf("%d %s %v", w.Code, w.Body.String(), err)
	}
	for _, tt := range []struct{ method, path, body string }{{"POST", "/parse", `{"text":"x","extra":true}`}, {"POST", "/reviews", `{"id":"bad","cardId":1,"known":true,"reverse":false}`}, {"GET", "/stats?days=0", ""}, {"GET", "/stats?days=bad", ""}, {"DELETE", "/cards/0", ""}, {"PUT", "/groups/-1", `{}`}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
		if w.Code != 400 {
			t.Fatalf("%s: %d %s", tt.path, w.Code, w.Body.String())
		}
	}
}
