package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type TimeFilter struct {
	DepartAfter  int
	ArriveAfter  int
	ArriveBefore int
}

type Config struct {
	From, To             string
	Dates                []string
	ReturnFrom, ReturnTo string
	ReturnDates          []string
	Adults, Results      int
	Direct, JSON         bool
	MaxDuration, Timeout time.Duration
	Outbound, Return     TimeFilter
}

func newTimeFilter() TimeFilter {
	return TimeFilter{DepartAfter: -1, ArriveAfter: -1, ArriveBefore: -1}
}

func parseDateList(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if _, err := time.Parse(time.DateOnly, part); err != nil {
			return nil, fmt.Errorf("invalid date %q: use YYYY-MM-DD", part)
		}
		if !seen[part] {
			out = append(out, part)
			seen[part] = true
		}
	}
	return out, nil
}

func parseClock(value string) (int, error) {
	if value == "" {
		return -1, nil
	}
	t, err := time.Parse("15:04", value)
	if err != nil {
		return -1, fmt.Errorf("invalid local time %q: use HH:MM", value)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func parseCLI(args []string, usageOut io.Writer) (Config, bool, error) {
	cfg := Config{Adults: 1, Results: 5, Timeout: 45 * time.Second, Outbound: newTimeFilter(), Return: newTimeFilter()}
	fs := flag.NewFlagSet("flightclaw", flag.ContinueOnError)
	fs.SetOutput(usageOut)
	var date, dates, ret, retDates, maxDuration, timeout string
	var od, oa, ob, rd, rb string
	fs.StringVar(&cfg.From, "from", "LGW", "departure airport or city code")
	fs.StringVar(&cfg.From, "f", "LGW", "departure airport or city code (shorthand)")
	fs.StringVar(&cfg.To, "to", "FNC", "destination airport or city code")
	fs.StringVar(&cfg.To, "t", "FNC", "destination airport or city code (shorthand)")
	fs.StringVar(&date, "date", "", "outbound date (YYYY-MM-DD)")
	fs.StringVar(&date, "d", "", "outbound date (shorthand)")
	fs.StringVar(&dates, "dates", "", "comma-separated outbound dates")
	fs.StringVar(&ret, "return", "", "return date (YYYY-MM-DD)")
	fs.StringVar(&ret, "r", "", "return date (shorthand)")
	fs.StringVar(&retDates, "return-dates", "", "comma-separated return dates")
	fs.StringVar(&cfg.ReturnFrom, "return-from", "", "return origin (defaults to --to)")
	fs.StringVar(&cfg.ReturnTo, "return-to", "", "return destination (defaults to --from)")
	fs.IntVar(&cfg.Results, "results", 5, "number of itineraries")
	fs.IntVar(&cfg.Adults, "adults", 1, "number of adults")
	fs.BoolVar(&cfg.Direct, "direct", false, "nonstop flights only")
	fs.StringVar(&maxDuration, "max-duration", "", "maximum duration per journey (for example 4h)")
	fs.StringVar(&od, "outbound-depart-after", "", "outbound local departure after HH:MM")
	fs.StringVar(&oa, "outbound-arrive-after", "", "outbound local arrival after HH:MM")
	fs.StringVar(&ob, "outbound-arrive-before", "", "outbound local arrival before HH:MM")
	fs.StringVar(&rd, "return-depart-after", "", "return local departure after HH:MM")
	fs.StringVar(&rb, "return-arrive-before", "", "return local arrival before HH:MM")
	fs.BoolVar(&cfg.JSON, "json", false, "emit JSON only")
	fs.StringVar(&timeout, "timeout", "45s", "overall search timeout")
	fs.Usage = func() {
		fmt.Fprintln(usageOut, "flightclaw - search live Google Flights headline fares")
		fmt.Fprintln(usageOut, "Usage: flightclaw --from CODE --to CODE (--date DATE | --dates DATES) [flags]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return cfg, true, nil
		}
		return cfg, false, err
	}
	if len(args) == 0 {
		fs.Usage()
		return cfg, true, nil
	}
	if fs.NArg() != 0 {
		return cfg, false, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	var err error
	if cfg.From, err = normalizeCode(cfg.From); err != nil {
		return cfg, false, err
	}
	if cfg.To, err = normalizeCode(cfg.To); err != nil {
		return cfg, false, err
	}
	if date != "" && dates != "" {
		return cfg, false, fmt.Errorf("--date and --dates are mutually exclusive")
	}
	if date == "" && dates == "" {
		return cfg, false, fmt.Errorf("one of --date or --dates is required")
	}
	if date != "" {
		dates = date
	}
	if cfg.Dates, err = parseDateList(dates); err != nil {
		return cfg, false, err
	}
	if ret != "" && retDates != "" {
		return cfg, false, fmt.Errorf("--return and --return-dates are mutually exclusive")
	}
	if ret != "" {
		retDates = ret
	}
	if cfg.ReturnDates, err = parseDateList(retDates); err != nil {
		return cfg, false, err
	}
	if cfg.ReturnFrom == "" {
		cfg.ReturnFrom = cfg.To
	}
	if cfg.ReturnTo == "" {
		cfg.ReturnTo = cfg.From
	}
	if cfg.ReturnFrom, err = normalizeCode(cfg.ReturnFrom); err != nil {
		return cfg, false, err
	}
	if cfg.ReturnTo, err = normalizeCode(cfg.ReturnTo); err != nil {
		return cfg, false, err
	}
	if cfg.Adults <= 0 {
		return cfg, false, fmt.Errorf("--adults must be positive")
	}
	if cfg.Results <= 0 {
		return cfg, false, fmt.Errorf("--results must be positive")
	}
	if timeout == "" {
		timeout = "45s"
	}
	if cfg.Timeout, err = time.ParseDuration(timeout); err != nil || cfg.Timeout <= 0 {
		return cfg, false, fmt.Errorf("--timeout must be a positive Go duration")
	}
	if maxDuration != "" {
		if cfg.MaxDuration, err = time.ParseDuration(maxDuration); err != nil || cfg.MaxDuration <= 0 {
			return cfg, false, fmt.Errorf("--max-duration must be a positive Go duration")
		}
	}
	if cfg.Outbound.DepartAfter, err = parseClock(od); err != nil {
		return cfg, false, err
	}
	if cfg.Outbound.ArriveAfter, err = parseClock(oa); err != nil {
		return cfg, false, err
	}
	if cfg.Outbound.ArriveBefore, err = parseClock(ob); err != nil {
		return cfg, false, err
	}
	if cfg.Return.DepartAfter, err = parseClock(rd); err != nil {
		return cfg, false, err
	}
	if cfg.Return.ArriveBefore, err = parseClock(rb); err != nil {
		return cfg, false, err
	}
	if len(cfg.ReturnDates) > 0 {
		validPair := false
		for _, out := range cfg.Dates {
			for _, back := range cfg.ReturnDates {
				if back >= out {
					validPair = true
				}
			}
		}
		if !validPair {
			return cfg, false, fmt.Errorf("all return dates precede every outbound date")
		}
	}
	return cfg, false, nil
}

type providerFactory func() (FlightProvider, error)

func run(args []string, stdout, stderr io.Writer, factory providerFactory) int {
	var usage bytes.Buffer
	cfg, help, err := parseCLI(args, &usage)
	if help {
		_, _ = io.Copy(stdout, &usage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "flightclaw: %v\n", err)
		return 2
	}
	provider, err := factory()
	if err != nil {
		fmt.Fprintf(stderr, "flightclaw: %v\n", err)
		return 1
	}
	response, err := searchFlights(context.Background(), provider, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "flightclaw: %v\n", err)
		return 1
	}
	if cfg.JSON {
		err = renderJSON(stdout, response)
	} else {
		err = renderHuman(stdout, response)
	}
	if err != nil {
		fmt.Fprintf(stderr, "flightclaw: write output: %v\n", err)
		return 1
	}
	return 0
}
