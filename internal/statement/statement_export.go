package statement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Appointment struct {
	ID           string `json:"id"`
	OccurredOn   string `json:"occurred_on"`
	Status       string `json:"status"`
	AmountCents  int64  `json:"amount_cents"`
	BillingReady bool   `json:"billing_ready"`
}

type ExportRequest struct {
	AccountID    string        `json:"account_id"`
	PeriodStart  string        `json:"period_start"`
	PeriodEnd    string        `json:"period_end"`
	Appointments []Appointment `json:"appointments"`
}

type ExportResult struct {
	StatementID  string          `json:"statement_id"`
	PDF          json.RawMessage `json:"pdf"`
	Notification Notification    `json:"notification"`
}

type Notification struct {
	Kind        string `json:"kind"`
	StatementID string `json:"statement_id"`
	Message     string `json:"message"`
}

type API interface {
	UsageTimeseries(context.Context, string, string) (json.RawMessage, error)
	GeneratePDF(context.Context, string, string) (json.RawMessage, error)
}

type Exporter struct{ api API }

func NewExporter(api API) *Exporter { return &Exporter{api: api} }

func (e *Exporter) Export(ctx context.Context, input ExportRequest) (ExportResult, error) {
	if input.AccountID == "" || input.PeriodStart == "" || input.PeriodEnd == "" {
		return ExportResult{}, errors.New("account_id, period_start, and period_end are required")
	}
	appointments := eligibleAppointments(input.Appointments)
	if len(appointments) == 0 {
		return ExportResult{}, errors.New("statement has no completed, billing-ready appointments")
	}
	usage, err := e.api.UsageTimeseries(ctx, input.PeriodStart, input.PeriodEnd)
	if err != nil {
		return ExportResult{}, err
	}
	statementID := stableStatementID(input)
	markdown, err := renderStatement(statementID, input, appointments, usage)
	if err != nil {
		return ExportResult{}, err
	}
	pdf, err := e.api.GeneratePDF(ctx, markdown, statementID)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{
		StatementID: statementID,
		PDF:         pdf,
		Notification: Notification{
			Kind:        "statement_ready",
			StatementID: statementID,
			Message:     "Your account statement is ready.",
		},
	}, nil
}

func eligibleAppointments(all []Appointment) []Appointment {
	result := make([]Appointment, 0, len(all))
	for _, appointment := range all {
		if appointment.Status == "completed" && appointment.BillingReady {
			result = append(result, appointment)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].OccurredOn == result[j].OccurredOn {
			return result[i].ID < result[j].ID
		}
		return result[i].OccurredOn < result[j].OccurredOn
	})
	return result
}

func stableStatementID(input ExportRequest) string {
	sum := sha256.Sum256([]byte(input.AccountID + "\x00" + input.PeriodStart + "\x00" + input.PeriodEnd))
	return "stmt_" + hex.EncodeToString(sum[:8])
}

func renderStatement(id string, input ExportRequest, appointments []Appointment, usage json.RawMessage) (string, error) {
	var formattedUsage bytes.Buffer
	if err := json.Indent(&formattedUsage, usage, "", "  "); err != nil {
		return "", fmt.Errorf("format usage data: %w", err)
	}
	var rows strings.Builder
	var total int64
	for _, appointment := range appointments {
		total += appointment.AmountCents
		fmt.Fprintf(&rows, "| %s | %s | %.2f |\n", appointment.ID, appointment.OccurredOn, float64(appointment.AmountCents)/100)
	}
	return fmt.Sprintf("# Account statement\n\nStatement: `%s`  \nPeriod: %s to %s\n\n## Completed appointments\n\n| Appointment | Date | Amount |\n|---|---|---:|\n%s\n**Appointment total: %.2f**\n\n## Metered account usage\n\n```json\n%s\n```\n", id, input.PeriodStart, input.PeriodEnd, rows.String(), float64(total)/100, formattedUsage.String()), nil
}
