# flightclaw

CLI tool for searching flights using Google Flights via the `fli` Python library.

## Installation

```bash
go build -o flightclaw .
```

## Usage

```bash
./flightclaw --from LGW --to FNC --date 2026-03-15
./flightclaw -f LHR -t JFK -d 2026-04-01 -r 2026-04-10 --adults 2
./flightclaw --from SFO --to NRT --date 2026-05-20 --results 10
```

## Options

- `-f, --from`     Departure airport (IATA code, e.g., LGW, LHR, SFO)
- `-t, --to`       Destination airport (IATA code, e.g., FNC, JFK, NRT)
- `-d, --date`     Departure date (YYYY-MM-DD) — required
- `-r, --return`   Return date for round trip (YYYY-MM-DD) — optional
- `--results`      Number of results to display (default: 5)
- `--adults`       Number of adult passengers (default: 1)

## Examples

### One-way flight
```bash
flightclaw --from LGW --to FNC --date 2026-03-15
```

### Round-trip flight
```bash
flightclaw -f LHR -t JFK -d 2026-04-01 -r 2026-04-10
```

### Multiple results
```bash
flightclaw --from SFO --to NRT --date 2026-05-20 --results 10
```

## Notes

- Airport codes must be valid IATA codes (3 uppercase letters)
- Dates must be in YYYY-MM-DD format
- Requires Python 3 and the `fli` library: `pip install fli`
