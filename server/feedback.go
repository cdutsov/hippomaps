package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Feedback from the page: a problem, a mistake of the map, an idea (the menu: Feedback; a link under a search or a
// route that failed):
//
//	POST {base}/api/feedback   {"kind": "bug", "text": "…", "email": "", "lang": "de", "ctx": {…}}   → 204
//
// kind bug, map, idea or other; text up to 4000 characters; email optional, for an answer; ctx optional (the page adds
// it unless the visitor turns it off): a link to what was on the screen (the search, the place or the route, and the
// map), the map style, the size of the screen, the browser or the app, and the last errors of the page.
//
// Each message is a file of FeedbackDir (JSON, mode 600), deleted after a year; no IP address is kept. With the SMTP
// settings also an e-mail to FeedbackTo, with the address of the visitor as Reply-To (sent in the background, three
// tries). 400 for a bad message (JSON {"error": "text" | "email" | "kind"}), 429 more than 5 an hour from one visitor,
// 503 when the file cannot be written. `hippomaps feedback` prints the last ones.

const (
	feedbackMaxText   = 4000 // characters of the message
	feedbackMaxErrors = 10   // errors of the page
	feedbackKeep      = 365 * 24 * time.Hour
	feedbackPerHour   = 5
)

// FeedbackKinds: the kinds of messages, with their names in the e-mail
var FeedbackKinds = map[string]string{"bug": "Problem", "map": "Wrong on the map", "idea": "Idea", "other": "Other"}

var feedbackLangs = map[string]bool{"en": true, "de": true, "fr": true, "it": true}

// Feedback is one message, as kept in its file.
type Feedback struct {
	ID    string       `json:"id"` // 20261115-142233-a1b2c3: the time (UTC) and a random part; the name of the file
	At    time.Time    `json:"at"`
	Kind  string       `json:"kind"`
	Text  string       `json:"text"`
	Email string       `json:"email,omitempty"`
	Lang  string       `json:"lang,omitempty"`
	Ctx   *FeedbackCtx `json:"ctx,omitempty"`
}

// FeedbackCtx is what the page adds to a message.
type FeedbackCtx struct {
	Link   string   `json:"link,omitempty"`   // the page as it was: the search, place or route, and the map (#map=)
	Style  string   `json:"style,omitempty"`  // the map style
	Screen string   `json:"screen,omitempty"` // 390x844@3
	Agent  string   `json:"agent,omitempty"`  // the browser, or the app for Android (hippomaps-android/1.4.1)
	Errors []string `json:"errors,omitempty"` // the last errors of the page, oldest first
}

func (s *Server) feedbackOn() bool { return s.cfg.FeedbackDir != "" }

// mailOn: the messages also go by e-mail
func (s *Server) mailOn() bool {
	return s.cfg.FeedbackTo != "" && s.cfg.SMTPHost != "" && s.cfg.SMTPUser != ""
}

func (s *Server) feedback(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var in struct {
		Kind  string       `json:"kind"`
		Text  string       `json:"text"`
		Email string       `json:"email"`
		Lang  string       `json:"lang"`
		Ctx   *FeedbackCtx `json:"ctx"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		feedbackError(w, "json")
		return
	}
	now := time.Now().UTC()
	f, bad := s.cleanFeedback(in.Kind, in.Text, in.Email, in.Lang, in.Ctx, now)
	if bad != "" {
		feedbackError(w, bad)
		return
	}
	if !s.feedbackPerIP.allow(visitor(r), now) {
		http.Error(w, "too many", http.StatusTooManyRequests)
		return
	}
	if err := saveFeedback(s.cfg.FeedbackDir, f); err != nil {
		s.log.Error("feedback", "err", err)
		http.Error(w, "not kept", http.StatusServiceUnavailable)
		return
	}
	s.log.Info("feedback", "id", f.ID, "kind", f.Kind, "chars", utf8.RuneCountInString(f.Text), "email", f.Email != "")
	if s.mailOn() {
		select {
		case s.mails <- f:
		default:
			s.log.Warn("feedback mail", "id", f.ID, "err", "queue full") // kept in its file all the same
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func feedbackError(w http.ResponseWriter, what string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": what})
}

// cleanFeedback checks a message and cleans its texts; bad names the field that is wrong
func (s *Server) cleanFeedback(kind, text, email, lang string, ctx *FeedbackCtx, now time.Time) (f Feedback, bad string) {
	if FeedbackKinds[kind] == "" {
		return f, "kind"
	}
	text = cleanText(text, true)
	if text == "" || utf8.RuneCountInString(text) > feedbackMaxText {
		return f, "text"
	}
	email = strings.TrimSpace(email)
	if email != "" && !goodEmail(email) {
		return f, "email"
	}
	if !feedbackLangs[lang] {
		lang = ""
	}
	f = Feedback{ID: feedbackID(now), At: now, Kind: kind, Text: text, Email: email, Lang: lang}
	if ctx != nil {
		c := &FeedbackCtx{Link: s.ownLink(ctx.Link), Style: cut(cleanText(ctx.Style, false), 40),
			Screen: cut(cleanText(ctx.Screen, false), 40), Agent: cut(cleanText(ctx.Agent, false), 400)}
		errs := ctx.Errors
		if len(errs) > feedbackMaxErrors {
			errs = errs[len(errs)-feedbackMaxErrors:]
		}
		for _, e := range errs {
			if e = cut(cleanText(e, false), 400); e != "" {
				c.Errors = append(c.Errors, e)
			}
		}
		if c.Link != "" || c.Style != "" || c.Screen != "" || c.Agent != "" || len(c.Errors) > 0 {
			f.Ctx = c
		}
	}
	return f, ""
}

// ownLink: a link to this site (PublicURL; without it any http(s) link), else nothing; a mail with the message must not
// carry links of others
func (s *Server) ownLink(link string) string {
	link = cleanText(link, false)
	u, err := url.Parse(link)
	if err != nil || len(link) > 4000 || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	if s.cfg.PublicURL != "" {
		own, err := url.Parse(s.cfg.PublicURL)
		if err != nil || !strings.EqualFold(u.Host, own.Host) {
			return ""
		}
	}
	return link
}

// cleanText: valid UTF-8 without control and direction characters, trimmed; lines: line breaks kept (\r\n as \n),
// else a line break is a space
func cleanText(s string, lines bool) string {
	s = strings.ToValidUTF8(strings.ReplaceAll(s, "\r\n", "\n"), "")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' && lines:
			return r
		case r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ' ':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r):
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// cut: at most n characters
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// goodEmail: one plain address with a domain that has a dot, no name, no comment
func goodEmail(e string) bool {
	if len(e) > 254 || strings.ContainsAny(e, " <>\"(),;:\\[]") {
		return false
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Name != "" || a.Address != e {
		return false
	}
	at := strings.LastIndexByte(e, '@')
	return at > 0 && strings.Contains(e[at+1:], ".") && !strings.HasSuffix(e, ".")
}

func feedbackID(t time.Time) string {
	b := make([]byte, 3)
	rand.Read(b)
	return t.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// saveFeedback writes the message to its file (through a temporary one) and deletes those older than a year
func saveFeedback(dir string, f Feedback) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".feedback-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(append(b, '\n'))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o600)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, f.ID+".json"))
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	pruneFeedback(dir, f.At)
	return nil
}

// pruneFeedback deletes the messages of more than a year ago (by the time in the name of the file)
func pruneFeedback(dir string, now time.Time) {
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || len(name) < 15 {
			continue
		}
		if t, err := time.Parse("20060102-150405", name[:15]); err == nil && now.Sub(t) > feedbackKeep {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

// ReadFeedback reads the messages kept in dir, the oldest first.
func ReadFeedback(dir string) ([]Feedback, error) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Feedback
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var f Feedback
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// FormatFeedback writes a message as text: the body of its e-mail and what `hippomaps feedback` prints.
func FormatFeedback(f Feedback) string {
	var b strings.Builder
	lang := ""
	if f.Lang != "" {
		lang = ", page in " + f.Lang
	}
	fmt.Fprintf(&b, "%s, %s UTC%s\n", FeedbackKinds[f.Kind], f.At.UTC().Format("2006-01-02 15:04"), lang)
	if f.Email != "" {
		fmt.Fprintf(&b, "From: %s\n", f.Email)
	} else {
		b.WriteString("No e-mail address given\n")
	}
	b.WriteString("\n" + f.Text + "\n")
	if c := f.Ctx; c != nil {
		b.WriteString("\n")
		for _, x := range [][2]string{{"Page", c.Link}, {"Map style", c.Style}, {"Screen", c.Screen}, {"Browser", c.Agent}} {
			if x[1] != "" {
				fmt.Fprintf(&b, "%s: %s\n", x[0], x[1])
			}
		}
		if len(c.Errors) > 0 {
			b.WriteString("Errors of the page:\n")
			for _, e := range c.Errors {
				b.WriteString("- " + e + "\n")
			}
		}
	} else {
		b.WriteString("\nNo details of the page (turned off)\n")
	}
	return b.String()
}

// feedbackMail is the e-mail of a message: from and to us, Reply-To the visitor when they gave an address
func feedbackMail(f Feedback, from, to string) []byte {
	subject := FeedbackKinds[f.Kind] + ": " + cut(strings.Join(strings.Fields(f.Text), " "), 70)
	domain := from[strings.LastIndexByte(from, '@')+1:]
	var b bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	h("From", (&mail.Address{Name: "hippomaps feedback", Address: from}).String())
	h("To", to)
	if f.Email != "" {
		h("Reply-To", f.Email)
	}
	h("Subject", mime.QEncoding.Encode("utf-8", "hippomaps – "+subject))
	h("Date", f.At.Format(time.RFC1123Z))
	h("Message-ID", "<feedback-"+f.ID+"@"+domain+">")
	h("MIME-Version", "1.0")
	h("Content-Type", "text/plain; charset=utf-8")
	h("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	q := quotedprintable.NewWriter(&b)
	body := FormatFeedback(f) + "\n-- \nhippomaps feedback " + f.ID + ", kept on the server for a year\n"
	q.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n")))
	q.Close()
	return b.Bytes()
}

// mailer sends the messages of the queue one after the other, each up to three times
func (s *Server) mailer() {
	for f := range s.mails {
		msg := feedbackMail(f, s.cfg.SMTPUser, s.cfg.FeedbackTo)
		var err error
		for try := 1; try <= 3; try++ {
			if err = s.sendMail(s.cfg.FeedbackTo, msg); err == nil {
				break
			}
			s.log.Warn("feedback mail", "id", f.ID, "try", try, "err", err)
			if try < 3 {
				time.Sleep(time.Duration(try) * s.mailRetry)
			}
		}
		if err == nil {
			s.log.Info("feedback mailed", "id", f.ID)
		}
	}
}

// smtpSend sends msg to one address: port 465 with TLS from the start, else STARTTLS (a server without it only on
// the loopback, for tests)
func (s *Server) smtpSend(to string, msg []byte) error {
	host, port := s.cfg.SMTPHost, s.cfg.SMTPPort
	if port == "" {
		port = "587"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(60 * time.Second))
	if port == "465" {
		conn = tls.Client(conn, &tls.Config{ServerName: host})
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if _, ok := conn.(*tls.Conn); !ok {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		} else if !plainSMTP(host) {
			return errors.New("the mail server offers no STARTTLS")
		}
	}
	if s.cfg.SMTPPassword != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, host)); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := c.Mail(s.cfg.SMTPUser); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// plainSMTP: a mail server without TLS only on the loopback (a test); elsewhere the password would go in the clear
func plainSMTP(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}
