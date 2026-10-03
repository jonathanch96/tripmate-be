package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMoneyAcceptsNumbersAndStringsExactly(t *testing.T) {
	var in struct {
		A Money `json:"a"`
		B Money `json:"b"`
		C Money `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a": 0.1, "b": "45000", "c": null}`), &in); err != nil {
		t.Fatal(err)
	}
	a, _ := in.A.Decimal("a")
	b, _ := in.B.Decimal("b")
	if a.String() != "0.1" || b.String() != "45000" || !in.C.IsZero() {
		t.Fatalf("parsed %s %s %q", a, b, in.C)
	}
	if _, err := Money("12,50").Decimal("x"); err == nil {
		t.Fatal("a comma-formatted amount must be refused, not guessed at")
	}
}

func TestDecimalInRefusesDotGroupedWholeAmountsInZeroDecimalCurrencies(t *testing.T) {
	if _, err := Money("45.000").DecimalIn("amount", "IDR"); err == nil || !strings.Contains(err.Error(), "45000") {
		t.Fatalf("IDR 45.000 = %v", err)
	}
	if value, err := Money("45.000").DecimalIn("amount", "USD"); err != nil || value.String() != "45" {
		t.Fatalf("USD 45.000 = %s, %v", value, err)
	}
	if value, err := Money("1250000").DecimalIn("amount", "IDR"); err != nil || value.String() != "1250000" {
		t.Fatalf("IDR 1250000 = %s, %v", value, err)
	}
}

func TestRosterResolveRefusesStrangersAndDeduplicates(t *testing.T) {
	ana, budi := uuid.New(), uuid.New()
	names := roster{ana: "Ana", budi: "Budi"}
	ids, err := names.resolve("user_ids", []string{ana.String(), ana.String(), budi.String()})
	if err != nil || len(ids) != 2 {
		t.Fatalf("ids = %v, %v", ids, err)
	}
	if _, err := names.resolve("user_ids", []string{uuid.NewString()}); err == nil || !strings.Contains(err.Error(), "not a participant") {
		t.Fatalf("stranger = %v", err)
	}
	if _, err := names.resolve("user_ids", []string{"Ana"}); err == nil || !strings.Contains(err.Error(), "not a user_id") {
		t.Fatalf("name instead of id = %v", err)
	}
}

// Every tool must register with a valid schema; AddTool panics otherwise, which would take the
// whole API down at startup.
func TestToolsRegisterWithSchemasAndAnnotations(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "tripmate", Version: "test"}, &mcp.ServerOptions{Instructions: Instructions})
	(&controller{}).registerTools(server)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if !strings.Contains(session.InitializeResult().Instructions, "get_active_trip") {
		t.Fatal("instructions should describe the bill workflow")
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	readOnly := map[string]bool{}
	for _, tool := range tools.Tools {
		readOnly[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}
	for name, want := range map[string]bool{"list_trips": true, "get_active_trip": true, "get_trip": true, "list_expenses": true,
		"get_balances": true, "list_settlements": true, "create_bill_expense": false, "add_expense": false,
		"record_settlement": false, "set_exchange_rate": false, "create_trip": false, "invite_participant": false} {
		got, ok := readOnly[name]
		if !ok || got != want {
			t.Errorf("tool %s registered=%v readOnly=%v, want readOnly=%v", name, ok, got, want)
		}
	}
	// A tool call without a bearer token must be refused rather than act as anyone.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_trips", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("an unauthenticated tool call must fail")
	}
}
