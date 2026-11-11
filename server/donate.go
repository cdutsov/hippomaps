package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Donations through Stripe Checkout (StripeKey: a restricted key that may write Checkout Sessions):
//
//	POST {base}/api/donate   amount=10&lang=de   {"url": "https://checkout.stripe.com/…"}
//
// amount in whole francs, minDonation to maxDonation; lang en, de, fr or it, for the checkout page and the name of the
// item. A donation, not a sale: no Managed Payments, the button of Stripe says Donate, and the card statement
// "OVERPASS* HIPPOMAPS". Back on the page after paying with ?donated=1. 400 for a bad amount, 429 too many,
// 502 when Stripe fails. Needs PublicURL for the way back.

const (
	minDonation = 2    // CHF
	maxDonation = 1000 // CHF
)

var donationName = map[string]string{"en": "Donation to HippoMaps", "de": "Spende an HippoMaps", "fr": "Don à HippoMaps", "it": "Donazione a HippoMaps"}

func (s *Server) donations() bool { return s.cfg.StripeKey != "" && s.cfg.PublicURL != "" }

func (s *Server) donate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	amount, err := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("amount")))
	if err != nil || amount < minDonation || amount > maxDonation {
		http.Error(w, "amount: whole francs from 2 to 1000", http.StatusBadRequest)
		return
	}
	lang := r.PostForm.Get("lang")
	if donationName[lang] == "" {
		lang = "en"
	}
	if !s.donatesPerIP.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many", http.StatusTooManyRequests)
		return
	}
	back := strings.TrimRight(s.cfg.PublicURL, "/") + s.home() // https://hippomaps.ch/
	f := url.Values{
		"mode":                                   {"payment"},
		"submit_type":                            {"donate"},
		"locale":                                 {lang},
		"line_items[0][quantity]":                {"1"},
		"line_items[0][price_data][currency]":    {"chf"},
		"line_items[0][price_data][unit_amount]": {strconv.Itoa(amount * 100)},
		"line_items[0][price_data][product_data][name]":    {donationName[lang]},
		"payment_intent_data[description]":                 {donationName["en"]},
		"payment_intent_data[statement_descriptor_suffix]": {"HIPPOMAPS"},
		"payment_intent_data[metadata][app]":               {"hippomaps"},
		"metadata[app]":                                    {"hippomaps"},
		"success_url":                                      {back + "?donated=1"},
		"cancel_url":                                       {back},
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.StripeAPI, "/")+"/v1/checkout/sessions", strings.NewReader(f.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.StripeKey, "")
	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Warn("donate", "err", err)
		http.Error(w, "failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var out struct {
		ID    string `json:"id"`
		URL   string `json:"url"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if err != nil || resp.StatusCode != http.StatusOK || !strings.HasPrefix(out.URL, "https://checkout.stripe.com/") {
		s.log.Warn("donate", "status", resp.StatusCode, "err", err, "stripe", out.Error.Message)
		http.Error(w, "failed", http.StatusBadGateway)
		return
	}
	s.log.Info("donate", "session", out.ID, "amount", amount, "lang", lang)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{"url": out.URL})
}
