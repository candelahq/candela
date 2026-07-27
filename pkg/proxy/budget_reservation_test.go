package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

func TestBudgetReservation(t *testing.T) {
	tests := []struct {
		name        string
		reqBody     string
		wantReserve float64 // if -1, we assert > 0.05
	}{
		{
			name:        "Small request uses floor of $0.05",
			reqBody:     `{"model":"claude-haiku-4.5","messages":[{"role":"user","content":"hi"}]}`,
			wantReserve: 0.05,
		},
		{
			name:        "Large request uses estimated cost",
			reqBody:     `{"model":"claude-opus-4.7","messages":[{"role":"user","content":"` + strings.Repeat("hello ", 50000) + `"}]}`,
			wantReserve: -1, // We will check > 0.05
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inFlightChan := make(chan struct{})
			resumeChan := make(chan struct{})

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				// Signal that the request has reached upstream (reservation made)
				close(inFlightChan)
				// Wait to resume so we can inspect pending spend
				<-resumeChan
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
			}))
			defer upstream.Close()

			submitter := &mockSubmitter{}
			calc := costcalc.New()

			p, _ := New(Config{
				Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
				ProjectID: "test",
			}, submitter, calc)

			userID := "test-budget-user@example.com"
			p.SetUserStore(&budgetUserStore{
				checkResult: &storage.BudgetCheckResult{
					Allowed:      true,
					RemainingUSD: 100.0,
				},
			})

			mux := http.NewServeMux()
			p.RegisterRoutes(mux)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := auth.NewContext(r.Context(), &auth.User{ID: userID, Email: userID})
				mux.ServeHTTP(w, r.WithContext(ctx))
			})
			srv := httptest.NewServer(handler)
			defer srv.Close()

			req, _ := http.NewRequest("POST",
				srv.URL+"/proxy/anthropic/v1/messages",
				strings.NewReader(tt.reqBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer tok")

			errCh := make(chan error, 1)
			go func() {
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					errCh <- err
					return
				}
				defer func() { _ = resp.Body.Close() }()
				_, _ = io.ReadAll(resp.Body)
				if resp.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("bad status: %d", resp.StatusCode)
					return
				}
				errCh <- nil
			}()

			// Wait for request to reach upstream
			<-inFlightChan

			// Check reservation amount
			got := p.pendingSpend.Get(userID)
			if tt.wantReserve == -1 {
				if got <= 0.05 {
					t.Errorf("expected reservation > 0.05, got %f", got)
				}
			} else {
				if got != tt.wantReserve {
					t.Errorf("expected reservation %f, got %f", tt.wantReserve, got)
				}
			}

			// Resume upstream and wait for completion
			close(resumeChan)
			if err := <-errCh; err != nil {
				t.Fatalf("request failed: %v", err)
			}

			// Check reservation is released
			assertEventually(t, func() bool {
				return p.pendingSpend.Get(userID) == 0
			}, 2*time.Second, "reservation leaked")
		})
	}
}
