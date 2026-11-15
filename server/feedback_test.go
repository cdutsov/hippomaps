package server

import (
	"bufio"
	"encoding/json"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func postJSON(h http.Handler, u string, body any, hdr ...string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, u, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func feedbackFiles(t *testing.T, dir string) []Feedback {
	t.Helper()
	all, err := ReadFeedback(dir)
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// A message is kept in a file of its own (mode 600), its texts cleaned, without the IP address; a link to another site
// and errors beyond the last 10 are dropped; the page has data-feedback.
func TestFeedback(t *testing.T) {
	dir := t.TempDir()
	s := testServer(t, nil, func(c *Config) { c.Base, c.PublicURL, c.FeedbackDir = "/", "https://hippomaps.ch", dir })
	if !strings.Contains(get(s, "/").Body.String(), `data-feedback="/api/feedback"`) {
		t.Fatal("page has no data-feedback")
	}
	errs := make([]string, 14)
	for i := range errs {
		errs[i] = "error " + string(rune('a'+i))
	}
	w := postJSON(s, "/api/feedback", map[string]any{
		"kind": "bug", "text": "  The route\r\nwent\x07 wrong\u202e  ", "email": "ana@example.org", "lang": "de",
		"ctx": map[string]any{"link": "https://hippomaps.ch/?q=caf%C3%A9s#map=15/47.37/8.54", "style": "light, 3D", "screen": "390x844@3",
			"agent": "Mozilla/5.0\n(Linux; Android 14) hippomaps-android/1.4.1", "errors": errs},
	}, "X-Forwarded-For", "203.0.113.9")
	if w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	all := feedbackFiles(t, dir)
	if len(all) != 1 {
		t.Fatalf("%d messages", len(all))
	}
	f := all[0]
	if f.Kind != "bug" || f.Text != "The route\nwent wrong" || f.Email != "ana@example.org" || f.Lang != "de" || time.Since(f.At) > time.Minute {
		t.Errorf("message %+v", f)
	}
	if f.Ctx == nil || f.Ctx.Link != "https://hippomaps.ch/?q=caf%C3%A9s#map=15/47.37/8.54" || f.Ctx.Style != "light, 3D" ||
		f.Ctx.Agent != "Mozilla/5.0 (Linux; Android 14) hippomaps-android/1.4.1" || len(f.Ctx.Errors) != 10 || f.Ctx.Errors[0] != "error e" {
		t.Errorf("ctx %+v", f.Ctx)
	}
	name := filepath.Join(dir, f.ID+".json")
	st, err := os.Stat(name)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("file %v %v", st, err)
	}
	if b, _ := os.ReadFile(name); strings.Contains(string(b), "203.0.113") {
		t.Error("the IP address is in the file")
	}
	// a link to another site is dropped; without ctx none
	postJSON(s, "/api/feedback", map[string]any{"kind": "idea", "text": "x", "ctx": map[string]any{"link": "https://evil.example/hippomaps.ch"}})
	postJSON(s, "/api/feedback", map[string]any{"kind": "other", "text": "no details"})
	all = feedbackFiles(t, dir)
	if len(all) != 3 || all[1].Ctx != nil || all[2].Ctx != nil || all[2].Text != "no details" {
		t.Errorf("%+v", all[1:])
	}
	// the text for the command and the e-mail
	out := FormatFeedback(f)
	for _, want := range []string{"Problem, ", "page in de", "From: ana@example.org", "The route\nwent wrong", "Page: https://hippomaps.ch/?q=", "Errors of the page:\n- error e"} {
		if !strings.Contains(out, want) {
			t.Errorf("format lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(FormatFeedback(all[2]), "No details of the page") {
		t.Error("format without details")
	}
}

func TestFeedbackBad(t *testing.T) {
	dir := t.TempDir()
	s := testServer(t, nil, func(c *Config) { c.Base, c.FeedbackDir = "/", dir })
	for _, c := range []struct {
		body any
		err  string
	}{
		{map[string]any{"kind": "spam", "text": "x"}, "kind"},
		{map[string]any{"kind": "bug", "text": " \n\t "}, "text"},
		{map[string]any{"kind": "bug", "text": strings.Repeat("ä", feedbackMaxText+1)}, "text"},
		{map[string]any{"kind": "bug", "text": "x", "email": "not an address"}, "email"},
		{map[string]any{"kind": "bug", "text": "x", "email": "a@b.ch\r\nBcc: c@d.ch"}, "email"},
		{map[string]any{"kind": "bug", "text": "x", "email": "Ana <a@b.ch>"}, "email"},
		{map[string]any{"kind": "bug", "text": "x", "email": "a@localhost"}, "email"},
		{"[1, 2]", "json"},
	} {
		w := postJSON(s, "/api/feedback", c.body)
		var out map[string]string
		json.Unmarshal(w.Body.Bytes(), &out)
		if w.Code != 400 || out["error"] != c.err {
			t.Errorf("%v: %d %v, want %s", c.body, w.Code, out, c.err)
		}
	}
	// the longest message passes
	if w := postJSON(s, "/api/feedback", map[string]any{"kind": "bug", "text": strings.Repeat("ä", feedbackMaxText)}); w.Code != 204 {
		t.Errorf("longest: %d", w.Code)
	}
	if w := get(s, "/api/feedback"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	// without FeedbackDir: no feedback
	off := testServer(t, nil, func(c *Config) { c.Base = "/" })
	if w := postJSON(off, "/api/feedback", map[string]any{"kind": "bug", "text": "x"}); w.Code == 204 {
		t.Error("feedback without a directory")
	}
	if strings.Contains(get(off, "/").Body.String(), "data-feedback") {
		t.Error("page has data-feedback without the feature")
	}
}

// 5 an hour from a visitor (an IPv6 network /64 is one); then 429, others still pass
func TestFeedbackLimit(t *testing.T) {
	s := testServer(t, nil, func(c *Config) { c.Base, c.FeedbackDir = "/", t.TempDir() })
	for i := 0; i < feedbackPerHour; i++ {
		if w := postJSON(s, "/api/feedback", map[string]any{"kind": "bug", "text": "x"}, "X-Forwarded-For", "2001:db8::"+string(rune('1'+i))); w.Code != 204 {
			t.Fatalf("%d: %d", i, w.Code)
		}
	}
	if w := postJSON(s, "/api/feedback", map[string]any{"kind": "bug", "text": "x"}, "X-Forwarded-For", "2001:db8::99"); w.Code != 429 {
		t.Errorf("sixth: %d", w.Code)
	}
	if w := postJSON(s, "/api/feedback", map[string]any{"kind": "bug", "text": "x"}, "X-Forwarded-For", "198.51.100.1"); w.Code != 204 {
		t.Errorf("another visitor: %d", w.Code)
	}
}

// messages of more than a year ago go when a new one comes; other files stay
func TestFeedbackPrune(t *testing.T) {
	dir := t.TempDir()
	old := Feedback{ID: "20240101-000000-aaaaaa", At: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Kind: "bug", Text: "old"}
	youngAt := time.Now().UTC().Add(-300 * 24 * time.Hour)
	young := Feedback{ID: youngAt.Format("20060102-150405") + "-bbbbbb", At: youngAt, Kind: "bug", Text: "young"}
	for _, f := range []Feedback{old, young} {
		b, _ := json.Marshal(f)
		os.WriteFile(filepath.Join(dir, f.ID+".json"), b, 0o600)
	}
	os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o600)
	if err := saveFeedback(dir, Feedback{ID: feedbackID(time.Now()), At: time.Now().UTC(), Kind: "idea", Text: "new"}); err != nil {
		t.Fatal(err)
	}
	all := feedbackFiles(t, dir)
	if len(all) != 2 || all[0].Text != "young" || all[1].Text != "new" {
		t.Errorf("%+v", all)
	}
	if _, err := os.Stat(filepath.Join(dir, "README")); err != nil {
		t.Error("another file deleted")
	}
}

// The e-mail of a message: from and to us, Reply-To the visitor, the subject encoded, the text quoted-printable; sent in
// the background, tried again after a failure
func TestFeedbackMail(t *testing.T) {
	var mu sync.Mutex
	var sent [][]byte
	fails := 1
	got := make(chan struct{}, 4)
	s := testServer(t, nil, func(c *Config) {
		c.Base, c.FeedbackDir, c.FeedbackTo, c.SMTPHost, c.SMTPUser = "/", t.TempDir(), "info@example.org", "mail.example.org", "info@example.org"
	})
	s.mailRetry = time.Millisecond
	s.sendMail = func(to string, msg []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if fails > 0 {
			fails--
			return io.ErrUnexpectedEOF
		}
		if to != "info@example.org" {
			t.Errorf("to %s", to)
		}
		sent = append(sent, msg)
		got <- struct{}{}
		return nil
	}
	text := "Die Strasse über den Klausenpass fehlt.\nZweite Zeile " + strings.Repeat("lang ", 300)
	if w := postJSON(s, "/api/feedback", map[string]any{"kind": "map", "text": text, "email": "ana@example.org", "lang": "de"}); w.Code != 204 {
		t.Fatal(w.Code)
	}
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("no mail")
	}
	mu.Lock()
	raw := sent[0]
	mu.Unlock()
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	subj, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if m.Header.Get("Reply-To") != "ana@example.org" || !strings.HasPrefix(subj, "hippomaps – Wrong on the map: Die Strasse über den Klausenpass fehlt. Zweite") ||
		!strings.Contains(m.Header.Get("From"), "<info@example.org>") || m.Header.Get("To") != "info@example.org" {
		t.Errorf("headers %v (subject %q)", m.Header, subj)
	}
	body, _ := io.ReadAll(quotedprintable.NewReader(m.Body))
	if !strings.Contains(string(body), "Die Strasse über den Klausenpass fehlt.\r\nZweite Zeile") || !strings.Contains(string(body), "From: ana@example.org") {
		t.Errorf("body %s", body)
	}
	for _, l := range strings.Split(string(raw), "\r\n") {
		if len(l) > 998 {
			t.Fatal("a line over 998 characters")
		}
	}
}

// smtpSend to a mail server on the loopback without STARTTLS: the message arrives as written
func TestSMTPSend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	data := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		say := func(s string) { io.WriteString(c, s+"\r\n") }
		say("220 fake")
		var msg strings.Builder
		for in := false; ; {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if in {
				if l == ".\r\n" {
					in = false
					data <- msg.String()
					say("250 ok")
					continue
				}
				msg.WriteString(l)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(l)); {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250-fake")
				say("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH PLAIN"):
				say("235 ok")
			case cmd == "DATA":
				in = true
				say("354 go")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	s := testServer(t, nil, func(c *Config) {
		c.SMTPHost, c.SMTPPort, c.SMTPUser, c.SMTPPassword = "127.0.0.1", port, "info@example.org", "secret"
	})
	msg := feedbackMail(Feedback{ID: "20261115-120000-abcdef", At: time.Now(), Kind: "idea", Text: "Mehr Velowege"}, "info@example.org", "info@example.org")
	if err := s.smtpSend("info@example.org", msg); err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-data:
		if !strings.Contains(d, "Mehr Velowege") || !strings.Contains(d, "Message-ID: <feedback-20261115-120000-abcdef@example.org>") {
			t.Errorf("data %s", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing arrived")
	}
	// a mail server elsewhere without STARTTLS: refused (the password would go in the clear)
	for host, want := range map[string]bool{"127.0.0.1": true, "::1": true, "localhost": true, "mail.infomaniak.com": false, "10.0.0.1": false, "localhost.example": false} {
		if plainSMTP(host) != want {
			t.Errorf("plainSMTP(%s) = %v", host, !want)
		}
	}
}
