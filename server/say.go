package server

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Spoken navigation from a text-to-speech server (TTSURL: Piper):
//
//	GET {base}/api/say?lang=de&text=…[&format=wav]   audio/mpeg, or audio/wav for browsers without MP3
//
// lang is one of sayLangs (a voice of Piper each), the text at most 400 characters (the instructions of Valhalla). The
// browser keeps each answer a week; this server keeps the latest in memory, as navigation says
// the same sentences again and again. 400 for a bad request, 429 too many, 503 busy, 502 failed;
// the page then speaks with the voices of the browser.

var sayLangs = map[string]bool{"bg": true, "de": true, "el": true, "en": true, "es": true, "et": true, "fi": true, "fr": true, "hu": true,
	"it": true, "nb": true, "nl": true, "pl": true, "pt": true, "ro": true, "ru": true, "sv": true, "uk": true}

// voiceList: the languages of sayLangs, for the page (data-voices)
func voiceList() string {
	l := make([]string, 0, len(sayLangs))
	for k := range sayLangs {
		l = append(l, k)
	}
	sort.Strings(l)
	return strings.Join(l, ",")
}

const maxSay = 400

func (s *Server) say(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang, text, format := q.Get("lang"), strings.Join(strings.Fields(q.Get("text")), " "), q.Get("format")
	if format == "" {
		format = "mp3"
	}
	if !sayLangs[lang] || text == "" || utf8.RuneCountInString(text) > maxSay || (format != "mp3" && format != "wav") {
		http.Error(w, "lang one of "+voiceList()+", a text of at most 400 characters, format mp3 or wav", http.StatusBadRequest)
		return
	}
	key := "say\n" + format + "\n" + lang + "\n" + text
	if b, ok := s.cache.get(key); ok {
		sendAudio(w, b, format)
		return
	}
	if !s.saysPerIP.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many", http.StatusTooManyRequests)
		return
	}
	select {
	case s.sayCalls <- struct{}{}:
		defer func() { <-s.sayCalls }()
	default:
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.TTSURL+"/say?"+url.Values{"lang": {lang}, "text": {text}, "format": {format}}.Encode(), nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		s.log.Warn("say", "err", err)
		http.Error(w, "failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "audio/") {
		s.log.Warn("say", "status", resp.StatusCode, "err", err)
		http.Error(w, "failed", http.StatusBadGateway)
		return
	}
	s.cache.put(key, b)
	sendAudio(w, b, format)
}

func sendAudio(w http.ResponseWriter, b []byte, format string) {
	h := w.Header()
	h.Set("Content-Type", map[string]string{"mp3": "audio/mpeg", "wav": "audio/wav"}[format])
	h.Set("Cache-Control", "public, max-age=604800")
	w.Write(b)
}
