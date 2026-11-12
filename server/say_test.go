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
	if !strings.Contains(get(s, "/labs/maps").Body.String(), `data-say="/labs/maps/api/say" data-voices="bg,de,el,en,es,et,fi,fr,hu,it,nb,nl,pl,pt,ro,ru,sv,uk"`) {
		t.Fatalf("page has no data-say with the voices")
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
	for _, u := range []string{"/labs/maps/api/say?lang=tr&text=Merhaba", "/labs/maps/api/say?lang=de&text=Hallo&format=ogg", "/labs/maps/api/say?lang=de&text=", "/labs/maps/api/say?lang=de&text=" + strings.Repeat("a", 401)} {
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

// Each language of a voice is a language of the steps (the voice says them), and a voice of Bulgarian is asked
// for in Bulgarian.
func TestSayLanguages(t *testing.T) {
	for l := range sayLangs {
		if languages[l] == "" {
			t.Errorf("voice %s: no language of the steps", l)
		}
	}
	var calls atomic.Int32
	tts := fakeTTS(t, &calls)
	s := testServer(t, nil, func(c *Config) { c.TTSURL = tts.URL })
	if w := get(s, "/labs/maps/api/say?lang=bg&text=%D0%97%D0%B0%D0%B2%D0%B8%D0%B9%D1%82%D0%B5+%D0%BD%D0%B0%D0%B4%D1%8F%D1%81%D0%BD%D0%BE."); w.Code != 200 || w.Body.String() != "ID3 bg Завийте надясно." {
		t.Errorf("bg: %d %q", w.Code, w.Body)
	}
}

// all the calls to the voice taken: 503 at once, the page then speaks with a voice of the browser; a
// sentence in memory is still served
func TestSayBusy(t *testing.T) {
	var calls atomic.Int32
	tts := fakeTTS(t, &calls)
	s := testServer(t, nil, func(c *Config) { c.TTSURL = tts.URL })
	if w := get(s, "/labs/maps/api/say?lang=de&text=Hallo"); w.Code != 200 {
		t.Fatalf("say: %d", w.Code)
	}
	for i := 0; i < cap(s.sayCalls); i++ {
		s.sayCalls <- struct{}{}
	}
	if w := get(s, "/labs/maps/api/say?lang=de&text=Neu"); w.Code != 503 || calls.Load() != 1 {
		t.Errorf("busy: %d, %d calls", w.Code, calls.Load())
	}
	if w := get(s, "/labs/maps/api/say?lang=de&text=Hallo"); w.Code != 200 {
		t.Errorf("a sentence in memory while busy: %d", w.Code)
	}
}

// the voice answers something that is no sound: 502, and it is not kept
func TestSayNotAudio(t *testing.T) {
	var calls atomic.Int32
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `<html>an error page with status 200</html>`)
	}))
	t.Cleanup(tts.Close)
	s := testServer(t, nil, func(c *Config) { c.TTSURL = tts.URL })
	for i := 0; i < 2; i++ {
		if w := get(s, "/labs/maps/api/say?lang=de&text=Hallo"); w.Code != 502 {
			t.Errorf("no sound: %d %q", w.Code, w.Body)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("calls: %d, want 2 (not kept)", calls.Load())
	}
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	s = testServer(t, nil, func(c *Config) { c.TTSURL = down.URL })
	if w := get(s, "/labs/maps/api/say?lang=de&text=Hallo"); w.Code != 502 {
		t.Errorf("no voice server: %d", w.Code)
	}
}
