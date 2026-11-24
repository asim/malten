package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asim/malten/session"
)

func TestMuPayWireContract(t *testing.T) {
	var origin string
	var calls []string
	var foreignCheckout bool
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agent-token" {
			t.Error("missing agent token")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/agent/malten" {
			raw, _ := io.ReadAll(r.Body)
			var req muRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Error(err)
			}
			if req.Session != "paid-1" || req.Thread != "" || strings.Contains(string(raw), "public_only") || len(raw) > 32000 {
				t.Error("wrong paid request")
			}
			if len(req.Context.Messages) < 2 || req.Context.Messages[1].Author != "Host" {
				t.Error("missing room context")
			}
			io.WriteString(w, `{"text":"Through Mu","thread":"private-to-Mu"}`)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/agent/malten/pay" {
			var data map[string]any
			if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
				t.Error(err)
			}
			if data["reference"] != "booking" || data["participants"] != float64(3) || len(data) != 2 {
				t.Error(data)
			}
		}
		link := origin + "/pay/paid-1"
		if foreignCheckout {
			link = "https://elsewhere.example/pay/paid-1"
		}
		json.NewEncoder(w).Encode(session.Receipt{ID: "paid-1", Reference: "booking", Participants: 3, Amount: 900, Currency: "gbp", Minutes: 60, Status: "pending", CheckoutURL: link})
	}))
	defer mock.Close()
	origin = mock.URL
	a := &Agent{endpoint: origin + "/agent/malten", client: mock.Client(), token: "agent-token"}
	ctx := context.Background()
	if _, err := a.AskSession(ctx, "", "unscoped", nil); err == nil {
		t.Fatal("empty paid session reached Mu")
	}
	if p, err := a.Checkout(ctx, "booking", 3); err != nil || p.Amount != 900 {
		t.Fatal(p, err)
	}
	if _, err := a.Payment(ctx, "paid-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartSession(ctx, "paid-1"); err != nil {
		t.Fatal(err)
	}
	history := []session.Message{{Author: "Host", Text: "A reflection"}}
	for range 2 {
		if reply, err := a.AskSession(ctx, "paid-1", "A question", history); err != nil || reply != "Through Mu" {
			t.Fatal(reply, err)
		}
	}
	want := "POST /agent/malten/pay,GET /agent/malten/pay/paid-1,POST /agent/malten/pay/paid-1/start,POST /agent/malten,POST /agent/malten"
	if strings.Join(calls, ",") != want {
		t.Fatal(calls)
	}
	foreignCheckout = true
	if _, err := a.Checkout(ctx, "booking", 3); err == nil {
		t.Fatal("accepted a checkout on another origin")
	}
}
