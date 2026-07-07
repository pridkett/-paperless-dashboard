# paperless-dashboard

A single pane of glass for keeping your [Paperless-NGX](https://docs.paperless-ngx.com/)
documents up to date. Define checks like "a bill from Eversource every month"
or "tax receipts every year, usually filed in July," and the dashboard shows
which periods have documents and which are missing.

## Quick start

```sh
go build -o paperless-dashboard .
cp config.example.toml config.toml   # then edit it
./paperless-dashboard
```

Open http://127.0.0.1:8985 in your browser.

## Configuration

Settings resolve with this precedence (highest wins), and the startup output
warns when a higher-precedence source overrides a lower one:

1. Command-line flags: `--url`, `--token`, `--listen`, `--config`
2. Environment variables: `PAPERLESS_URL`, `PAPERLESS_TOKEN`, `PAPERLESS_DASHBOARD_LISTEN`
3. Config file (`config.toml` by default)
4. Built-in defaults

Checks are defined in the config file as `[[check]]` tables:

```toml
[[check]]
name = "Eversource Bill"         # display name (required)
correspondent = "Eversource"     # Paperless correspondent name
document_type = "Bill"           # Paperless document type name
tags = ["utility"]               # document must carry all of these tags
frequency = "monthly"            # monthly (default), quarterly, or yearly
lookback = 12                    # number of past periods to check
grace_days = 15                  # days into the current period before it's "missing"
custom_fields = ["amount"]       # custom field values shown from the newest document

[[check]]
name = "Tax Receipts"
document_type = "Tax"
frequency = "yearly"
expected_month = 7               # normally filed in July; current year stays
grace_days = 14                  # "pending" until July + 14 days has passed
```

Every period in the lookback window is classified as:

- **ok** — at least one matching document exists in that period
- **missing** — the period passed (or its grace window did) with no document
- **pending** — the current period, still within its grace window

See `config.example.toml` for the full annotated reference.

## API token

Create a token in Paperless-NGX under your user profile (Settings → API
token), then put it in the config file or `PAPERLESS_TOKEN`.

## Development

```sh
go test ./...
```
