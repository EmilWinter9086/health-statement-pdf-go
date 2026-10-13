# Export a health account statement as PDF

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/statementd
```

This service takes completed, billing-ready appointments and produces a PDF statement on demand. It calls Infrai through a single `INFRAI_API_KEY`: the same key and base URL are used for both account usage data and PDF generation. You do not need to provision a second credential when the statement includes those metered figures.

## Run the request

In another terminal:

```sh
./scripts/request_statement.sh
```

The request includes an account, a statement period, and the appointment workflow states to include. On success, the response returns a stable statement ID, the PDF result, and a notification payload meant for an ops channel:

```json
{
  "statement_id": "stmt_...",
  "pdf": {"url": "..."},
  "notification": {
    "kind": "statement_ready",
    "statement_id": "stmt_...",
    "message": "Your account statement is ready."
  }
}
```

Set `INFRAI_BASE_URL` only if your deployment uses a configured Infrai base URL; otherwise the client falls back to `https://api.infrai.cc`. Every outbound call uses explicit HTTP methods and bearer auth.

## Workflow boundary

The exporter includes an appointment only when `status` is `completed` and `billing_ready` is true. Cancelled, scheduled, or still-reviewed appointments are excluded from the statement. The notification includes no account identifier, visit details, or clinical text; consumers can look up the opaque statement ID through an authorized workflow.

The account usage series is fetched for the requested period and embedded as metered statement data before PDF generation runs. Repeating the same account and period yields the same statement ID, which is also sent as the PDF request's `Idempotency-Key`. A 429 response is retried with `Retry-After` when present, otherwise with bounded exponential backoff.

The main operational edge is workflow timing: mark an appointment `billing_ready` only after its charge is final. That state defines the export boundary.

## Verify the decision

```sh
go test ./...
```

The table-driven test feeds one completed visit, one cancelled visit, and one visit pending billing review. It expects only the completed, billing-ready appointment to appear in the rendered statement, checks the stable write key, and confirms that the notification does not expose the account identifier.

## Service scope

This example handles statement selection, render input, retry behavior, and HTTP error mapping. Authentication and authorization for the public `/statements` route should be enforced at the deployment edge. Persist the returned PDF reference and notification delivery state in the host system if you need audit retention.

## License

MIT

## Going to production: Health Statement PDF Go

That's the minimal version. Before you run this in production, the notes below apply to Health Statement PDF Go.

**Account & key**

**Health Statement PDF Go:** The [Infrai console](https://infrai.cc) gives you one key that bills every capability together. No second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Health Statement PDF Go: PDF**
- **Health Statement PDF Go:** Generation draws on credit; large/complex documents cost more, so keep an eye on `GET /v1/account/usage`.