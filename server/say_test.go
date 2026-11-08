package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func fakeTTS(t *testing.T, calls *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		if r.URL.Path != "/say" || q.Get("lang") == "" || q.Get("text") == "" {
			w.WriteHeader(400)
			return
		}
		if q.Get("text") == "fail" {
			w.WriteHeader(500)
			return
		}
		if q.Get("format") == "wav" {
			w.Header().Set("Content-Type", "audio/wav")
			io.WriteString(w, "RIFF "+q.Get("lang")+" "+q.Get("text"))
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "ID3 "+q.Get("lang")+" "+q.Get("text"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSay(t *testing.T) {
	var calls atomic.Int32
	tts := fakeTTS(t, &calls)
	s := testServer(t, nil, func(c *Config) { c.TTSURL = tts.URL + "/"; c.SaysPerIP = 3 })
	if !strings.Contains(get(s, "/labs/maps").Body.String(), `data-say="/labs/maps/api/say"`) {
		t.Fatalf("page has no data-say")
	}
	w := get(s, "/labs/maps/api/say?lang=de&text=Rechts+%20abbiegen.")
	if w.Code != 200 || w.Body.String() != "ID3 de Rechts abbiegen." || w.Header().Get("Content-Type") != "audio/mpeg" || !strings.Contains(w.Header().Get("Cache-Control"), "max-age") {
		t.Fatalf("say: %d %q %v", w.Code, w.Body, w.Header())
	}
	// the same sentence again comes from memory: no call, and not counted
	for i := 0; i < 3; i++ {
		get(s, "/labs/maps/api/say?lang=de&text=Rechts+abbiegen.")
	}
	if calls.Load() != 1 {
		t.Errorf("calls to the voice: %d", calls.Load())
	}
	// for a browser without MP3: WAV, kept apart from the MP3 of the same sentence
	w = get(s, "/labs/maps/api/say?lang=de&text=Rechts+abbiegen.&format=wav")
	if w.Code != 200 || w.Body.String() != "RIFF de Rechts abbiegen." || w.Header().Get("Content-Type") != "audio/wav" {
		t.Errorf("wav: %d %q %v", w.Code, w.Body, w.Header().Get("Content-Type"))
	}
	if w := get(s, "/labs/maps/api/say?lang=en&text=fail"); w.Code != 502 {
		t.Errorf("failed voice: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/say?lang=en&text=Turn+left."); w.Code != 429 {
		t.Errorf("third new sentence: %d", w.Code)
	}
	n := calls.Load()
	for _, u := range []string{"/labs/maps/api/say?lang=es&text=Hola", "/labs/maps/api/say?lang=de&text=Hallo&format=ogg", "/labs/maps/api/say?lang=de&text=", "/labs/maps/api/say?lang=de&text=" + strings.Repeat("a", 401)} {
		if w := get(s, u); w.Code != 400 {
			t.Errorf("%.60s: %d", u, w.Code)
		}
	}
	if calls.Load() != n {
		t.Errorf("the voice was asked for a bad request")
	}
}

func TestSayOff(t *testing.T) {
	s := testServer(t, nil)
	if w := get(s, "/labs/maps/api/say?lang=de&text=Hallo"); w.Code != 404 {
		t.Errorf("say without TTSURL: %d", w.Code)
	}
	if strings.Contains(get(s, "/labs/maps").Body.String(), "data-say") {
		t.Errorf("page has data-say without TTSURL")
	}
}
