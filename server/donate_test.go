package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeStripe answers POST /v1/checkout/sessions like Stripe and keeps the form of the last call
func fakeStripe(t *testing.T, calls *atomic.Int32, last *url.Values, fail bool) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		user, _, _ := r.BasicAuth()
		if r.Method != http.MethodPost || r.URL.Path != "/v1/checkout/sessions" || user != "rk_test_donate" {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":{"message":"bad call"}}`)
			return
		}
		r.ParseForm()
		*last = r.PostForm
		if fail {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"no such thing"}}`)
			return
		}
		io.WriteString(w, `{"id":"cs_test_1","url":"https://checkout.stripe.com/c/pay/cs_test_1"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func postForm(h http.Handler, u string, form url.Values, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestDonate(t *testing.T) {
	var calls atomic.Int32
	var last url.Values
	st := fakeStripe(t, &calls, &last, false)
	s := testServer(t, nil, func(c *Config) {
		c.Base, c.PublicURL, c.StripeKey, c.StripeAPI = "/", "https://hippomaps.ch", "rk_test_donate", st.URL+"/"
	})
	if !strings.Contains(get(s, "/").Body.String(), `data-donate="/api/donate"`) {
		t.Fatal("page has no data-donate")
	}
	w := postForm(s, "/api/donate", url.Values{"amount": {"10"}, "lang": {"de"}})
	var out map[string]string
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out["url"] != "https://checkout.stripe.com/c/pay/cs_test_1" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("donate: %d %q %v", w.Code, w.Body, w.Header())
	}
	// a donation in francs, not a sale: Donate on the button of Stripe, no Managed Payments, back to the page with ?donated=1
	for k, v := range map[string]string{
		"mode": "payment", "submit_type": "donate", "locale": "de",
		"line_items[0][price_data][currency]": "chf", "line_items[0][price_data][unit_amount]": "1000", "line_items[0][quantity]": "1",
		"line_items[0][price_data][product_data][name]":    "Spende an HippoMaps",
		"payment_intent_data[statement_descriptor_suffix]": "HIPPOMAPS", "metadata[app]": "hippomaps",
		"success_url": "https://hippomaps.ch/?donated=1", "cancel_url": "https://hippomaps.ch/",
	} {
		if last.Get(k) != v {
			t.Errorf("%s: %q, want %q", k, last.Get(k), v)
		}
	}
	if last.Has("managed_payments[enabled]") || last.Has("automatic_tax[enabled]") {
		t.Errorf("a donation is no sale: %v", last)
	}
	// another language than the four: English
	postForm(s, "/api/donate", url.Values{"amount": {"25"}, "lang": {"xx"}})
	if last.Get("locale") != "en" || last.Get("line_items[0][price_data][product_data][name]") != "Donation to HippoMaps" || last.Get("line_items[0][price_data][unit_amount]") != "2500" {
		t.Errorf("english: %v", last)
	}
	// whole francs from 2 to 1000, else 400 and no call to Stripe
	n := calls.Load()
	for _, a := range []string{"", "1", "0", "-5", "1001", "10.50", "ten", "1e3"} {
		if w := postForm(s, "/api/donate", url.Values{"amount": {a}}); w.Code != 400 {
			t.Errorf("amount %q: %d", a, w.Code)
		}
	}
	if calls.Load() != n {
		t.Errorf("Stripe asked for a bad amount")
	}
	if w := get(s, "/api/donate?amount=10"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	// ten checkouts per visitor in ten minutes
	for i := 0; i < 8; i++ {
		postForm(s, "/api/donate", url.Values{"amount": {"10"}}, "X-Forwarded-For", "192.0.2.7")
	}
	if w := postForm(s, "/api/donate", url.Values{"amount": {"10"}}, "X-Forwarded-For", "192.0.2.7"); w.Code != 200 {
		t.Errorf("9th: %d", w.Code)
	}
	if w := postForm(s, "/api/donate", url.Values{"amount": {"10"}}, "X-Forwarded-For", "192.0.2.7"); w.Code != 200 {
		t.Errorf("10th: %d", w.Code)
	}
	if w := postForm(s, "/api/donate", url.Values{"amount": {"10"}}, "X-Forwarded-For", "192.0.2.7"); w.Code != http.StatusTooManyRequests {
		t.Errorf("11th: %d", w.Code)
	}
}

// Stripe refuses: 502, and the page shows its message
func TestDonateStripeFails(t *testing.T) {
	var calls atomic.Int32
	var last url.Values
	st := fakeStripe(t, &calls, &last, true)
	s := testServer(t, nil, func(c *Config) {
		c.Base, c.PublicURL, c.StripeKey, c.StripeAPI = "/", "https://hippomaps.ch", "rk_test_donate", st.URL
	})
	if w := postForm(s, "/api/donate", url.Values{"amount": {"10"}}); w.Code != http.StatusBadGateway || calls.Load() != 1 {
		t.Errorf("failed: %d, %d calls", w.Code, calls.Load())
	}
}

// without the key, or without the address of the site for the way back: no donations, and none on the page
func TestDonateOff(t *testing.T) {
	for _, c := range []func(*Config){
		func(c *Config) { c.PublicURL = "https://hippomaps.ch" },
		func(c *Config) { c.StripeKey = "rk_test_donate" },
	} {
		s := testServer(t, nil, c)
		if strings.Contains(get(s, "/labs/maps").Body.String(), "data-donate") {
			t.Error("page has data-donate")
		}
		if w := postForm(s, "/labs/maps/api/donate", url.Values{"amount": {"10"}}); w.Code != 404 && w.Code != 405 {
			t.Errorf("donate: %d", w.Code)
		}
	}
}
