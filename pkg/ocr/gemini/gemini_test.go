package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jblabs/tripmate-be/pkg/apperror"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
)

const secretKey = "super-secret-gemini-key"

func geminiReply(text string) string {
	body, _ := json.Marshal(map[string]any{
		"candidates": []map[string]any{{"content": map[string]any{"parts": []map[string]string{{"text": text}}}}},
	})
	return string(body)
}

func newTestProvider(t *testing.T, handler http.HandlerFunc) (*Provider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	provider := New(Config{APIKey: secretKey, Endpoint: server.URL, Timeout: 5 * time.Second})
	provider.sleep = func(time.Duration) {} // Do not actually wait out the backoff in tests.
	return provider, server
}

func image() receiptdomain.Image {
	return receiptdomain.Image{Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, ContentType: "image/jpeg"}
}

func TestExtractSendsTheImageAndReadsTheItemsBack(t *testing.T) {
	var gotBody []byte
	var gotPath, gotKey string
	provider, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotPath, gotKey = r.URL.Path, r.Header.Get("x-goog-api-key")
		fmt.Fprint(w, geminiReply(`{"merchant":"Cafe","currency":"USD","lineItems":[{"name":"Tea","quantity":"1","unitPrice":"3.50","totalPrice":"3.50"}],"tax":"0.50","serviceCharge":"0","total":"4.00"}`))
	})

	result, err := provider.Extract(context.Background(), image())
	if err != nil {
		t.Fatal(err)
	}
	if result.Merchant != "Cafe" || len(result.Items) != 1 || result.Total.String() != "4" {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(gotPath, defaultModel) {
		t.Fatalf("path = %q, want it to name the model", gotPath)
	}
	if gotKey != secretKey {
		t.Fatalf("the API key was not sent as a header")
	}
	// The image must actually be attached, base64'd, with its real media type.
	var sent map[string]any
	if err = json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotBody), "image/jpeg") || !strings.Contains(string(gotBody), "/9j/") {
		t.Fatalf("the request body does not carry the image: %s", gotBody)
	}
	if !strings.Contains(string(gotBody), "application/json") {
		t.Fatal("the request does not ask for a JSON response")
	}
}

// OCR-1: one retry on a 5xx, and the eventual answer is used.
func TestExtractRetriesOnceOnServerError(t *testing.T) {
	var calls atomic.Int32
	provider, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, geminiReply(`{"merchant":"Second Try","currency":"USD","lineItems":[{"name":"Tea","quantity":"1","unitPrice":"3","totalPrice":"3"}],"total":"3"}`))
	})

	result, err := provider.Extract(context.Background(), image())
	if err != nil {
		t.Fatal(err)
	}
	if result.Merchant != "Second Try" {
		t.Fatalf("merchant = %q", result.Merchant)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want exactly 2 (one retry)", calls.Load())
	}
}

func TestExtractGivesUpAfterTheRetryWithAProviderError(t *testing.T) {
	var calls atomic.Int32
	provider, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := provider.Extract(context.Background(), image())
	if !apperror.Is(err, "OCR_PROVIDER_ERROR") {
		t.Fatalf("error = %v, want OCR_PROVIDER_ERROR", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2 — one attempt and one retry", calls.Load())
	}
}

// A 4xx is the caller's fault, so retrying it just burns quota.
func TestExtractDoesNotRetryAClientError(t *testing.T) {
	var calls atomic.Int32
	provider, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	})

	if _, err := provider.Extract(context.Background(), image()); !apperror.Is(err, "OCR_PROVIDER_ERROR") {
		t.Fatalf("error = %v, want OCR_PROVIDER_ERROR", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 — a 4xx must not be retried", calls.Load())
	}
}

// OCR-2: a response that is not a receipt fails as unparseable, distinctly from a provider outage.
func TestExtractReportsUnparseableSeparatelyFromAnOutage(t *testing.T) {
	for _, test := range []struct{ name, reply, want string }{
		{name: "prose instead of json", reply: geminiReply("I cannot read this receipt."), want: "OCR_UNPARSEABLE"},
		{name: "no candidates at all", reply: `{"candidates":[]}`, want: "OCR_UNPARSEABLE"},
		{name: "envelope is not json", reply: `<html>502 Bad Gateway</html>`, want: "OCR_UNPARSEABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, test.reply)
			})
			if _, err := provider.Extract(context.Background(), image()); !apperror.Is(err, test.want) {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
		})
	}
}

// OCR-6: whatever goes wrong, the key must not turn up in an error a user or a log could see.
func TestTheAPIKeyNeverAppearsInAnError(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "server error", handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{name: "rejected request", handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			// A provider that echoes the key back must not make it into our error either.
			fmt.Fprintf(w, `{"error":{"message":"API key %s is invalid"}}`, secretKey)
		}},
		{name: "garbage response", handler: func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "not json") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider, _ := newTestProvider(t, test.handler)
			_, err := provider.Extract(context.Background(), image())
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), secretKey) {
				t.Fatalf("the API key leaked into the error: %v", err)
			}
			if message := err.(*apperror.Error).Message; strings.Contains(message, secretKey) {
				t.Fatalf("the API key leaked into the message: %s", message)
			}
		})
	}
}

// The key must not ride in the query string, where it would land in access logs.
func TestTheAPIKeyIsNotPutInTheURL(t *testing.T) {
	var gotQuery string
	provider, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, geminiReply(`{"merchant":"X","currency":"USD","lineItems":[{"name":"T","quantity":"1","unitPrice":"1","totalPrice":"1"}],"total":"1"}`))
	})
	if _, err := provider.Extract(context.Background(), image()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotQuery, secretKey) {
		t.Fatalf("the key is in the query string: %s", gotQuery)
	}
}

func TestProviderIdentifiesItself(t *testing.T) {
	provider := New(Config{APIKey: "k"})
	if provider.Name() != "gemini" || provider.Model() != defaultModel {
		t.Fatalf("name/model = %s/%s", provider.Name(), provider.Model())
	}
	custom := New(Config{APIKey: "k", Model: "gemini-1.5-pro"})
	if custom.Model() != "gemini-1.5-pro" {
		t.Fatalf("model = %s", custom.Model())
	}
}

func TestExtractHonoursAContextThatIsAlreadyDone(t *testing.T) {
	provider, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, geminiReply(`{}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Extract(ctx, image()); !apperror.Is(err, "OCR_PROVIDER_ERROR") {
		t.Fatalf("error = %v, want OCR_PROVIDER_ERROR", err)
	}
}
