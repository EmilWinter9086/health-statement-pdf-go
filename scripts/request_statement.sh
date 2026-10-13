#!/bin/sh
set -eu

curl --fail-with-body --request POST http://localhost:8080/statements \
  --header 'Content-Type: application/json' \
  --data '{
    "account_id": "patient-account-42",
    "period_start": "2026-09-01",
    "period_end": "2026-09-30",
    "appointments": [
      {"id":"apt-1001","occurred_on":"2026-09-08","status":"completed","amount_cents":12500,"billing_ready":true},
      {"id":"apt-1002","occurred_on":"2026-09-12","status":"cancelled","amount_cents":8000,"billing_ready":true}
    ]
  }'
