package statement

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeAPI struct {
	usage    json.RawMessage
	markdown string
	key      string
}

func (f *fakeAPI) UsageTimeseries(context.Context, string, string) (json.RawMessage, error) {
	return f.usage, nil
}

func (f *fakeAPI) GeneratePDF(_ context.Context, markdown, key string) (json.RawMessage, error) {
	f.markdown, f.key = markdown, key
	return json.RawMessage(`{"url":"https://files.example.test/statement.pdf"}`), nil
}

func TestExportAppointmentDecision(t *testing.T) {
	tests := []struct {
		name         string
		appointments []Appointment
		wantIDs      []string
		wantErr      bool
	}{
		{"includes only completed billing-ready visits", []Appointment{
			{ID: "apt-complete", OccurredOn: "2026-09-02", Status: "completed", AmountCents: 12500, BillingReady: true},
			{ID: "apt-cancelled", OccurredOn: "2026-09-03", Status: "cancelled", AmountCents: 9900, BillingReady: true},
			{ID: "apt-review", OccurredOn: "2026-09-04", Status: "completed", AmountCents: 4500, BillingReady: false},
		}, []string{"apt-complete"}, false},
		{"refuses an empty billable period", []Appointment{
			{ID: "apt-scheduled", OccurredOn: "2026-09-05", Status: "scheduled", AmountCents: 8000, BillingReady: false},
		}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{usage: json.RawMessage(`{"series":[{"value":7}]}`)}
			result, err := NewExporter(api).Export(context.Background(), ExportRequest{
				AccountID: "patient-account-42", PeriodStart: "2026-09-01", PeriodEnd: "2026-09-30", Appointments: tt.appointments,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Export() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			for _, id := range tt.wantIDs {
				if !strings.Contains(api.markdown, id) {
					t.Errorf("statement does not contain eligible appointment %q", id)
				}
			}
			if strings.Contains(api.markdown, "apt-cancelled") || strings.Contains(api.markdown, "apt-review") {
				t.Error("statement contains an ineligible appointment")
			}
			if result.Notification.Message != "Your account statement is ready." || strings.Contains(result.Notification.Message, "patient-account-42") {
				t.Errorf("notification is not patient-safe: %#v", result.Notification)
			}
			if api.key != result.StatementID {
				t.Error("PDF idempotency key must match the stable statement ID")
			}
		})
	}
}
