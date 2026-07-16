package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestParseCLIValidatesAndNormalizesAllFlags(t *testing.T) {
	cfg, help, err := parseCLI([]string{
		"-f", " lon ", "-t", "rom", "--dates", "2026-09-13, 2026-09-14",
		"--return-from", "nap", "--return-to", "lon", "--return-dates", "2026-09-26,2026-09-27",
		"--adults", "2", "--results", "7", "--direct", "--max-duration", "4h",
		"--outbound-depart-after", "08:30", "--outbound-arrive-after", "12:00", "--outbound-arrive-before", "21:15",
		"--return-depart-after", "09:00", "--return-arrive-before", "23:00", "--timeout", "30s", "--json",
	}, io.Discard)
	if err != nil || help {
		t.Fatalf("parse = %#v, help=%v, err=%v", cfg, help, err)
	}
	if cfg.From != "LON" || cfg.To != "ROM" || cfg.ReturnFrom != "NAP" || cfg.ReturnTo != "LON" {
		t.Fatalf("codes not normalized: %#v", cfg)
	}
	if len(cfg.Dates) != 2 || len(cfg.ReturnDates) != 2 {
		t.Fatalf("dates not parsed: %#v", cfg)
	}
	if cfg.Adults != 2 || cfg.Results != 7 || !cfg.Direct || !cfg.JSON || cfg.MaxDuration != 4*time.Hour || cfg.Timeout != 30*time.Second {
		t.Fatalf("flags not parsed: %#v", cfg)
	}
	if cfg.Outbound.DepartAfter != 8*60+30 || cfg.Outbound.ArriveAfter != 12*60 || cfg.Outbound.ArriveBefore != 21*60+15 || cfg.Return.DepartAfter != 9*60 || cfg.Return.ArriveBefore != 23*60 {
		t.Fatalf("times not parsed: %#v", cfg)
	}
}

func TestParseCLIRejectsInvalidInputs(t *testing.T) {
	cases := [][]string{
		{"--from", "XX", "--to", "FCO", "--date", "2026-09-13"},
		{"--from", "LTN", "--to", "FCO"},
		{"--from", "LTN", "--to", "FCO", "--date", "bad"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13", "--dates", "2026-09-14"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13", "--return", "2026-09-20", "--return-dates", "2026-09-21"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13", "--adults", "0"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13", "--results", "0"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13", "--timeout", "0s"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13", "--max-duration", "nope"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13", "--outbound-depart-after", "24:00"},
		{"--from", "LTN", "--to", "FCO", "--date", "2026-09-20", "--return", "2026-09-19"},
	}
	for _, args := range cases {
		if _, _, err := parseCLI(args, io.Discard); err == nil {
			t.Errorf("expected invalid: %v", args)
		}
	}
}

type runProvider struct {
	search func(context.Context, LegQuery) ([]Journey, []string, error)
}

func (p runProvider) SearchOneWay(ctx context.Context, query LegQuery) ([]Journey, []string, error) {
	return p.search(ctx, query)
}

func TestRunExitCodesAndStreams(t *testing.T) {
	validArgs := []string{"--from", "LTN", "--to", "FCO", "--date", "2026-09-13"}
	tests := []struct {
		name     string
		args     []string
		factory  providerFactory
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{name: "help", args: []string{"--help"}, factory: func() (FlightProvider, error) { return nil, errors.New("must not initialize") }, wantCode: 0, wantOut: "Usage:"},
		{name: "validation", args: []string{"--from", "XX", "--to", "FCO", "--date", "2026-09-13"}, factory: func() (FlightProvider, error) { return nil, errors.New("must not initialize") }, wantCode: 2, wantErr: "invalid airport/city code"},
		{name: "provider initialization", args: validArgs, factory: func() (FlightProvider, error) { return nil, errors.New("session unavailable") }, wantCode: 1, wantErr: "session unavailable"},
		{name: "total runtime failure", args: validArgs, factory: func() (FlightProvider, error) {
			return runProvider{search: func(context.Context, LegQuery) ([]Journey, []string, error) { return nil, nil, errors.New("blocked") }}, nil
		}, wantCode: 1, wantErr: "all outbound flight searches failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr, tc.factory); code != tc.wantCode {
				t.Fatalf("exit = %d, want %d; stdout=%q stderr=%q", code, tc.wantCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantOut) || !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunPartialSuccessExitsZeroWithDeterministicWarning(t *testing.T) {
	provider := runProvider{search: func(_ context.Context, query LegQuery) ([]Journey, []string, error) {
		if query.Date == "2026-09-14" {
			return nil, nil, errors.New("temporary failure")
		}
		return []Journey{testJourney(query.Date, "LTN", "FCO", 25, 8)}, nil, nil
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--from", "LTN", "--to", "FCO", "--dates", "2026-09-13,2026-09-14"}, &stdout, &stderr, func() (FlightProvider, error) { return provider, nil })
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "2026-09-14 failed: temporary failure") || !strings.Contains(stdout.String(), "£25.00") {
		t.Fatalf("partial result/warning missing: %s", stdout.String())
	}
}
