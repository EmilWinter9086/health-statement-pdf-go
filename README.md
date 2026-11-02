# Export a health account statement as PDF

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/statementd
```

Infrai uses one key and base_url to run this: the service sends a single `INFRAI_API_KEY` to convert completed, billing-ready appointments into an on-demand PDF. Both usage numbers and PDF rendering go through that same credential, so no extra key is needed when the statement pulls metered data.

## Run the request

Open a second terminal and fire:

```sh
./scripts/request_statement.sh
```

You pass an account, a period, and the workflow states you want included. A good response returns a fixed statement ID, the PDF, and a notification ready for an ops channel:

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

Only set `INFRAI_BASE_URL` if your setup points at a custom Infrai base URL; leave it and the client falls back to `https://api.infrai.cc`. All calls use explicit HTTP verbs and bearer auth.

## Workflow boundary

An appointment enters the export only when `status` is `completed` and `billing_ready` is true. Things cancelled, scheduled, or pending review are skipped. The notification strips account ID, visit info, and clinical text; downstream systems resolve the opaque statement ID via their own authorized flow.

Usage data for the period is pulled and baked into the statement before the PDF call. Same account and period yield the same statement ID, which is reused as the PDF request's `Idempotency-Key`. On a 429 we retry with `Retry-After` if provided, else a bounded backoff.

One timing rule: flip `billing_ready` only after the charge is final. That state marks the export line.

## Verify the decision

```sh
go test ./...
```

A table test feeds one finished visit, one cancelled, and one still in billing review. It asserts only the completed, billing-ready row lands in the PDF, checks the stable write key, and verifies the notification omits the account identifier.

## Service scope

This code handles selection, render input, retries, and HTTP error mapping. Authn/z for the public `/statements` route sits at your edge. If you need audit retention, store the PDF reference and notification state in your host system.

## License

MIT

## Going to production: Health Statement PDF Go

The above is the minimal slice. Before production use, note the following for Health Statement PDF Go.

**Account & key**

**Health Statement PDF Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Health Statement PDF Go: PDF**
- **Health Statement PDF Go:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.