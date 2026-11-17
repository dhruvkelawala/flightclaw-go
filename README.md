# Flightclaw

Flightclaw is a pure-Go command-line client for comparing live Google Flights headline fares. It supports airport and city codes, multiple dates, independent one-way return searches (including open-jaw trips), filtering, complete segment details, and stable JSON output.

## Requirements and installation

Flightclaw requires Go 1.25 or later and has no additional language runtime dependency.

Build in the repository:

```bash
go build -o flightclaw ./cmd/flightclaw
./flightclaw --help
```

Or install it as `flightclaw` on your `GOBIN`/`GOPATH`:

```bash
go install github.com/dhruvkelawala/flightclaw-go/cmd/flightclaw@latest
```

## Usage

At least one outbound date is required:

```text
flightclaw --from CODE --to CODE (--date YYYY-MM-DD | --dates DATE,DATE) [flags]
```

Airport and city codes are case-insensitive three-letter IATA-style codes.

### Flags

| Flag | Shorthand | Description | Default |
|---|---:|---|---:|
| `--from` | `-f` | Outbound airport or city code | `LGW` |
| `--to` | `-t` | Outbound destination airport or city code | `FNC` |
| `--date` | `-d` | One outbound date (`YYYY-MM-DD`) | required unless `--dates` |
| `--dates` | | Comma-separated alternative outbound dates | |
| `--return` | `-r` | One independent return date | |
| `--return-dates` | | Comma-separated alternative return dates | |
| `--return-from` | | Return origin (for open-jaw trips) | outbound destination |
| `--return-to` | | Return destination (for open-jaw trips) | outbound origin |
| `--results` | | Maximum itineraries to print | `5` |
| `--adults` | | Adult travelers | `1` |
| `--direct` | | Nonstop journeys only | false |
| `--max-duration` | | Maximum duration for each one-way journey (Go duration, e.g. `4h`) | none |
| `--outbound-depart-after` | | Earliest local outbound departure (`HH:MM`) | none |
| `--outbound-arrive-after` | | Earliest local outbound arrival (`HH:MM`) | none |
| `--outbound-arrive-before` | | Latest local outbound arrival (`HH:MM`) | none |
| `--return-depart-after` | | Earliest local return departure (`HH:MM`) | none |
| `--return-arrive-before` | | Latest local return arrival (`HH:MM`) | none |
| `--json` | | Emit only structured JSON; warnings are embedded in it | false |
| `--timeout` | | Overall search timeout (Go duration) | `45s` |

`--date` and `--dates` are mutually exclusive, as are `--return` and `--return-dates`.

## Examples

One-way airport search:

```bash
flightclaw --from LTN --to FCO --date 2026-09-13 --results 3
```

Multiple outbound dates:

```bash
flightclaw --from LON --to ROM --dates 2026-09-13,2026-09-14 --results 10
```

Independent return search:

```bash
flightclaw -f LHR -t JFK -d 2026-09-13 -r 2026-09-26 --adults 2
```

Open-jaw trip with alternative return dates:

```bash
flightclaw --from LON --to ROM --date 2026-09-13 \
  --return-from NAP --return-to LON \
  --return-dates 2026-09-26,2026-09-27 --results 5
```

Direct flight with local-time and duration filters:

```bash
flightclaw --from LON --to ROM --date 2026-09-13 --direct \
  --outbound-depart-after 08:00 --outbound-arrive-after 15:00 \
  --outbound-arrive-before 22:00 --max-duration 4h
```

Automation-safe JSON:

```bash
flightclaw --from LTN --to FCO --date 2026-09-13 --results 1 --json | jq .
```

## City aliases

Flightclaw expands each city alias into one provider query per date:

- `LON`: LHR, LGW, STN, LTN, LCY, SEN
- `NYC`: JFK, LGA, EWR
- `PAR`: CDG, ORY, BVA
- `ROM`: FCO, CIA
- `MIL`: MXP, LIN, BGY
- `TYO`: HND, NRT
- `OSA`: KIX, ITM, UKB
- `SEL`: ICN, GMP
- `BJS`: PEK, PKX
- `SHA`: PVG, SHA
- `CHI`: ORD, MDW
- `WAS`: IAD, DCA, BWI
- `BUE`: EZE, AEP

## Fare and search caveats

- Results are live **headline fares** and can change between searches. They may exclude checked or cabin baggage, seat selection, payment, or other airline fees.
- Return and open-jaw totals are the sum of two independently searched one-way headline fares. They can differ from an airline's round-trip price.
- Flightclaw links to Google Flights for booking/search continuation; it does not book or purchase travel.
- Flightclaw reads the structured flight data embedded in Google's public search page. Google can change this format or block automated requests. HTTP failures, missing flight data and unreadable offers are reported as errors rather than as empty searches.
- Searches with no matching outbound or return flights exit with status `1` and write an error to stderr, including with `--json`. Invalid arguments exit with status `2`; successful searches exit with status `0`.

A separate scheduled live smoke test searches a high-availability route each day. It is intentionally kept out of pull-request CI: its purpose is to detect breakage in the Google Flights page data without making upstream availability a merge gate.

## License

Flightclaw is available under the [MIT License](LICENSE).
