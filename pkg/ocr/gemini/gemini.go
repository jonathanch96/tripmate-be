// Package gemini reads receipts with Google's Gemini models.
//
// It speaks the REST API directly rather than pulling in an SDK: the request is one JSON document
// and the response handling is the delicate part, which lives in pkg/ocr where it is covered by
// fixtures.
package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/ocr"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
)

const (
	defaultEndpoint = "https://generativelanguage.googleapis.com/v1beta"
	defaultModel    = "gemini-1.5-flash"
	// OCR-1: a receipt is a single image; thirty seconds is generous and bounds the request.
	defaultTimeout = 30 * time.Second
)

// prompt is ported from the prototype, which worked, with the three additions the sprint calls for:
// an ISO-4217 currency code, amounts as strings so the model stops reformatting floats, and no
// commentary around the JSON.
const prompt = `You are a receipt OCR expert. Analyze this receipt/bill image and extract ALL line items with their prices.

Return ONLY a valid JSON object with this exact structure:
{
  "merchant": "store/restaurant name",
  "date": "YYYY-MM-DD or null if not visible",
  "currency": "ISO-4217 code such as USD, PHP, EUR, IDR",
  "lineItems": [
    {
      "name": "item name/description",
      "quantity": "1",
      "unitPrice": "0.00",
      "totalPrice": "0.00"
    }
  ],
  "subtotal": "0.00",
  "tax": "0.00",
  "serviceCharge": "0.00",
  "total": "0.00"
}

Rules:
1. Extract EVERY line item visible on the receipt
2. If quantity is not shown, assume 1
3. Use the actual numbers from the receipt
4. If tax or service charge is shown, include it
5. Currency must be a three-letter ISO-4217 code detected from symbols or context (₱=PHP, $=USD, Rp=IDR, etc). If you cannot tell, use null
6. Give every amount as a STRING with a dot decimal separator and no currency symbol or thousands separator
7. Return ONLY the JSON, no explanations and no markdown fences`

type Config struct {
	APIKey   string
	Model    string
	Endpoint string
	Timeout  time.Duration
}

type Provider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
	sleep    func(time.Duration)
}

func New(cfg Config) *Provider {
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultEndpoint
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Provider{
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		endpoint: cfg.Endpoint,
		client:   &http.Client{Timeout: cfg.Timeout},
		sleep:    time.Sleep,
	}
}

func (p *Provider) Name() string  { return "gemini" }
func (p *Provider) Model() string { return p.model }

type request struct {
	Contents []content `json:"contents"`
	Config   genConfig `json:"generationConfig"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inline_data,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

// genConfig asks for JSON back, which is what stops the markdown fences at the source. The parser
// still copes with fences, because a model that promises JSON does not always deliver it.
type genConfig struct {
	ResponseMimeType string  `json:"response_mime_type"`
	Temperature      float64 `json:"temperature"`
}

type response struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (p *Provider) Extract(ctx context.Context, img receiptdomain.Image) (*receiptdomain.ExtractionResult, error) {
	body, err := json.Marshal(request{
		Contents: []content{{Parts: []part{
			{InlineData: &inlineData{MimeType: img.ContentType, Data: base64.StdEncoding.EncodeToString(img.Data)}},
			{Text: prompt},
		}}},
		Config: genConfig{ResponseMimeType: "application/json", Temperature: 0},
	})
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}

	// OCR-1: one retry on a transient failure, with jitter so a burst of uploads does not
	// synchronise its retries.
	var raw []byte
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			p.sleep(backoff(attempt))
		}
		raw, lastErr = p.call(ctx, body)
		if lastErr == nil {
			break
		}
		if !retryable(lastErr) {
			return nil, unwrapTransient(lastErr)
		}
	}
	if lastErr != nil {
		return nil, unwrapTransient(lastErr)
	}

	text, err := firstText(raw)
	if err != nil {
		return nil, err
	}
	result, err := ocr.Parse([]byte(text))
	if err != nil {
		// OCR-2: keep whatever the model said, so a failed receipt can be debugged later.
		return nil, err
	}
	return result, nil
}

func (p *Provider) call(ctx context.Context, body []byte) ([]byte, error) {
	// OCR-6: the key travels as a header, never in the URL, so it cannot leak through a logged
	// request line or an error containing the endpoint.
	url := fmt.Sprintf("%s/models/%s:generateContent", p.endpoint, p.model)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(request)
	if err != nil {
		// The error from the transport can contain the URL but never the header, so it is safe.
		return nil, apperror.Newf("OCR_PROVIDER_ERROR", "the receipt provider could not be reached")
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, apperror.Newf("OCR_PROVIDER_ERROR", "the receipt provider response could not be read")
	}
	if resp.StatusCode >= 500 {
		return nil, &transient{apperror.Newf("OCR_PROVIDER_ERROR", "the receipt provider is unavailable")}
	}
	if resp.StatusCode != http.StatusOK {
		// The provider's own message may quote the request; it is not passed through.
		return nil, apperror.Newf("OCR_PROVIDER_ERROR", "the receipt provider rejected the request")
	}
	return payload, nil
}

func firstText(raw []byte) (string, error) {
	var body response
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", apperror.Newf("OCR_UNPARSEABLE", "the receipt provider returned an unreadable envelope")
	}
	for _, candidate := range body.Candidates {
		for _, item := range candidate.Content.Parts {
			if item.Text != "" {
				return item.Text, nil
			}
		}
	}
	return "", apperror.Newf("OCR_UNPARSEABLE", "the receipt provider returned no content")
}

// transient marks the failures worth trying again: 5xx and timeouts. It never escapes Extract —
// callers see the plain OCR_PROVIDER_ERROR underneath.
type transient struct{ error }

func (t *transient) Unwrap() error { return t.error }

func retryable(err error) bool {
	var wrapped *transient
	return errors.As(err, &wrapped)
}

func unwrapTransient(err error) error {
	var wrapped *transient
	if errors.As(err, &wrapped) {
		return wrapped.error
	}
	return err
}

func backoff(attempt int) time.Duration {
	base := time.Duration(attempt) * 500 * time.Millisecond
	return base + time.Duration(rand.Int63n(int64(250*time.Millisecond)))
}
