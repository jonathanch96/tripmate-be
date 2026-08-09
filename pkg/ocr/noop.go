package ocr

import (
	"context"

	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
)

// fixture is a small, plausible bill. It has a shared-looking starter, two mains, tax and service —
// enough for the assignment grid and the proportional allocation to be worth looking at.
const fixture = `{
  "merchant": "Noop Diner",
  "date": "2026-09-15",
  "currency": "PHP",
  "lineItems": [
    {"name": "Calamari (to share)", "quantity": 1, "unitPrice": 480.00, "totalPrice": 480.00},
    {"name": "Grilled Tuna", "quantity": 1, "unitPrice": 620.00, "totalPrice": 620.00},
    {"name": "Chicken Adobo", "quantity": 1, "unitPrice": 540.00, "totalPrice": 540.00},
    {"name": "Rice", "quantity": 3, "unitPrice": 60.00, "totalPrice": 180.00},
    {"name": "San Miguel", "quantity": 4, "unitPrice": 95.00, "totalPrice": 380.00}
  ],
  "subtotal": 2200.00,
  "tax": 264.00,
  "serviceCharge": 220.00,
  "total": 2684.00
}`

// Noop is rule OCR-7: a provider that needs no API key, no network, and no quota. It is what runs
// in tests and in CI, and what a developer gets when GEMINI_API_KEY is empty — so the entire upload
// → extract → assign → convert flow is demoable offline.
type Noop struct{}

func NewNoop() *Noop { return &Noop{} }

func (n *Noop) Name() string  { return "noop" }
func (n *Noop) Model() string { return "fixture" }

func (n *Noop) Extract(_ context.Context, _ receiptdomain.Image) (*receiptdomain.ExtractionResult, error) {
	return Parse([]byte(fixture))
}
