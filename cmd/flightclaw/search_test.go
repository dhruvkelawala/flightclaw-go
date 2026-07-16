package main

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	mu       sync.Mutex
	calls    []LegQuery
	results  map[string][]Journey
	errors   map[string]error
	warnings map[string][]string
}

func (f *fakeProvider) SearchOneWay(ctx context.Context, query LegQuery) ([]Journey, []string, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}
	f.mu.Lock()
	f.calls = append(f.calls, query)
	f.mu.Unlock()
	return f.results[query.Date], f.warnings[query.Date], f.errors[query.Date]
}

func testJourney(date, from, to string, price float64, departHour int) Journey {
	day := map[string]int{"2026-09-13": 13, "2026-09-14": 14, "2026-09-19": 19, "2026-09-20": 20, "2026-09-26": 26, "2026-09-27": 27}[date]
	dep := time.Date(2026, 9, day, departHour, 0, 0, 0, time.UTC)
	return Journey{Date: date, Price: price, Currency: "GBP", DurationMinutes: 120, Segments: []Segment{{Airline: "Air", FlightNumber: "A1", DepartureAirport: from, ArrivalAirport: to, DepartureTime: dep, ArrivalTime: dep.Add(2 * time.Hour), DurationMinutes: 120}}}
}

func TestExpandLocationNormalizesEveryCityAlias(t *testing.T) {
	aliases := map[string][]string{
		"LON": {"LHR", "LGW", "STN", "LTN", "LCY", "SEN"},
		"NYC": {"JFK", "LGA", "EWR"},
		"PAR": {"CDG", "ORY", "BVA"},
		"ROM": {"FCO", "CIA"},
		"MIL": {"MXP", "LIN", "BGY"},
		"TYO": {"HND", "NRT"},
		"OSA": {"KIX", "ITM", "UKB"},
		"SEL": {"ICN", "GMP"},
		"BJS": {"PEK", "PKX"},
		"SHA": {"PVG", "SHA"},
		"CHI": {"ORD", "MDW"},
		"WAS": {"IAD", "DCA", "BWI"},
		"BUE": {"EZE", "AEP"},
	}
	for code, want := range aliases {
		got, err := expandLocation("  " + code + " ")
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", code, got, want)
		}
	}
	got, err := expandLocation(" ltn ")
	if err != nil || !reflect.DeepEqual(got, []string{"LTN"}) {
		t.Fatalf("airport normalization = %v, %v", got, err)
	}
	for _, invalid := range []string{"XX", "AB1", "ÅBC", "ABCD"} {
		if _, err := expandLocation(invalid); err == nil {
			t.Errorf("expected %q invalid", invalid)
		}
	}
}

func TestSearchUsesOneCityQueryPerOutboundDate(t *testing.T) {
	fake := &fakeProvider{results: map[string][]Journey{
		"2026-09-13": {testJourney("2026-09-13", "LTN", "FCO", 30, 8)},
		"2026-09-14": {testJourney("2026-09-14", "LGW", "CIA", 25, 9)},
	}, errors: map[string]error{}, warnings: map[string][]string{}}
	cfg := Config{From: "LON", To: "ROM", Dates: []string{"2026-09-13", "2026-09-14"}, Adults: 1, Results: 5, Timeout: time.Second, Outbound: newTimeFilter(), Return: newTimeFilter()}
	response, err := searchFlights(context.Background(), fake, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %d, want one per date: %#v", len(fake.calls), fake.calls)
	}
	for _, call := range fake.calls {
		if !reflect.DeepEqual(call.Origins, cityAliases["LON"]) || !reflect.DeepEqual(call.Destinations, cityAliases["ROM"]) {
			t.Fatalf("city not sent in one query: %#v", call)
		}
	}
	dates := []string{fake.calls[0].Date, fake.calls[1].Date}
	sort.Strings(dates)
	if !reflect.DeepEqual(dates, cfg.Dates) {
		t.Fatalf("dates = %v", dates)
	}
	if len(response.Results) != 2 || response.Results[0].TotalPrice != 25 || response.Query.CandidateCount != 2 {
		t.Fatalf("response = %#v", response)
	}
}

func TestSearchCombinesOpenJawDatesAndSkipsReturnBeforeOutbound(t *testing.T) {
	fake := &fakeProvider{results: map[string][]Journey{
		"2026-09-20": {testJourney("2026-09-20", "LTN", "FCO", 30, 8)},
		"2026-09-19": {testJourney("2026-09-19", "NAP", "LGW", 10, 10)},
		"2026-09-26": {testJourney("2026-09-26", "NAP", "LGW", 20, 11)},
	}, errors: map[string]error{}, warnings: map[string][]string{}}
	cfg := Config{From: "LON", To: "ROM", Dates: []string{"2026-09-20"}, ReturnFrom: "NAP", ReturnTo: "LON", ReturnDates: []string{"2026-09-19", "2026-09-26"}, Adults: 1, Results: 5, Timeout: time.Second, Outbound: newTimeFilter(), Return: newTimeFilter()}
	response, err := searchFlights(context.Background(), fake, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("calls = %#v", fake.calls)
	}
	if len(response.Results) != 1 || response.Results[0].Return == nil || response.Results[0].Return.Date != "2026-09-26" || response.Results[0].TotalPrice != 50 {
		t.Fatalf("invalid open-jaw combinations: %#v", response.Results)
	}
	if response.Query.ReturnFrom != "NAP" || response.Query.ReturnTo != "LON" {
		t.Fatalf("query summary lost open jaw: %#v", response.Query)
	}
}

func TestFilterJourneyChecksDirectDurationAndLocalTimes(t *testing.T) {
	journey := testJourney("2026-09-13", "LTN", "FCO", 30, 8)
	journey.DurationMinutes = 180
	journey.Segments[0].ArrivalTime = journey.Segments[0].DepartureTime.Add(3 * time.Hour)
	connecting := journey
	connecting.Stops = 1
	connecting.Segments = append(connecting.Segments, Segment{DepartureTime: journey.Segments[0].ArrivalTime, ArrivalTime: journey.Segments[0].ArrivalTime.Add(time.Hour)})
	cases := []struct {
		name      string
		candidate Journey
		direct    bool
		max       time.Duration
		filter    TimeFilter
		want      bool
	}{
		{"accept", journey, true, 3 * time.Hour, TimeFilter{DepartAfter: 8 * 60, ArriveAfter: 11 * 60, ArriveBefore: 11 * 60}, true},
		{"direct", connecting, true, 0, newTimeFilter(), false},
		{"duration", journey, false, 179 * time.Minute, newTimeFilter(), false},
		{"depart-after", journey, false, 0, TimeFilter{DepartAfter: 8*60 + 1, ArriveAfter: -1, ArriveBefore: -1}, false},
		{"arrive-after", journey, false, 0, TimeFilter{DepartAfter: -1, ArriveAfter: 11*60 + 1, ArriveBefore: -1}, false},
		{"arrive-before", journey, false, 0, TimeFilter{DepartAfter: -1, ArriveAfter: -1, ArriveBefore: 10*60 + 59}, false},
	}
	for _, tc := range cases {
		if got := journeyMatches(tc.candidate, tc.direct, tc.max, tc.filter); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSearchDeduplicatesAndSortsDeterministically(t *testing.T) {
	late := testJourney("2026-09-13", "LTN", "FCO", 25, 10)
	early := testJourney("2026-09-13", "LTN", "FCO", 25, 8)
	earlyTie := early
	earlyTie.Segments = append([]Segment(nil), early.Segments...)
	earlyTie.Segments[0].FlightNumber = "A0"
	fake := &fakeProvider{results: map[string][]Journey{"2026-09-13": {late, early, earlyTie, early}}, errors: map[string]error{}, warnings: map[string][]string{}}
	cfg := Config{From: "LTN", To: "FCO", Dates: []string{"2026-09-13"}, Adults: 1, Results: 5, Timeout: time.Second, Outbound: newTimeFilter(), Return: newTimeFilter()}
	response, err := searchFlights(context.Background(), fake, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 3 || response.Query.CandidateCount != 3 {
		t.Fatalf("duplicates retained: %#v", response)
	}
	first := response.Results[0].Outbound.Segments[0]
	if first.DepartureTime.Hour() != 8 || first.FlightNumber != "A0" {
		t.Fatalf("ties not sorted deterministically: %#v", response.Results)
	}
}

type concurrencyProvider struct {
	mu      sync.Mutex
	current int
	maximum int
	calls   int
	started chan struct{}
	release <-chan struct{}
}

func (p *concurrencyProvider) SearchOneWay(ctx context.Context, query LegQuery) ([]Journey, []string, error) {
	p.mu.Lock()
	p.current++
	p.calls++
	if p.current > p.maximum {
		p.maximum = p.current
	}
	p.mu.Unlock()
	select {
	case p.started <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
	case <-ctx.Done():
	}
	p.mu.Lock()
	p.current--
	p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return []Journey{testJourney("2026-09-13", "LTN", "FCO", 25, 8)}, nil, nil
}

func legQueries(count int) []LegQuery {
	queries := make([]LegQuery, count)
	for i := range queries {
		queries[i] = LegQuery{Origins: []string{"LTN"}, Destinations: []string{"FCO"}, Date: time.Date(2026, 9, 1+i, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), Adults: 1}
	}
	return queries
}

func TestSearchLegsBoundsConcurrency(t *testing.T) {
	release := make(chan struct{})
	provider := &concurrencyProvider{started: make(chan struct{}, 20), release: release}
	done := make(chan struct{})
	go func() {
		searchLegs(context.Background(), provider, legQueries(20))
		close(done)
	}()
	for i := 0; i < searchWorkerCount; i++ {
		select {
		case <-provider.started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	provider.mu.Lock()
	maximum := provider.maximum
	provider.mu.Unlock()
	if maximum > searchWorkerCount {
		t.Fatalf("maximum concurrency = %d, want <= %d", maximum, searchWorkerCount)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("search did not finish")
	}
}

func TestSearchLegsCancellationStopsOutstandingProviderCallsAndSortsWarnings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &concurrencyProvider{started: make(chan struct{}, 20), release: make(chan struct{})}
	type result struct {
		warnings []string
		failures int
	}
	done := make(chan result, 1)
	go func() {
		_, warnings, failures := searchLegs(ctx, provider, legQueries(20))
		done <- result{warnings: warnings, failures: failures}
	}()
	for i := 0; i < searchWorkerCount; i++ {
		select {
		case <-provider.started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	cancel()
	got := <-done
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls > searchWorkerCount {
		t.Fatalf("provider called %d times after cancellation, want <= %d", calls, searchWorkerCount)
	}
	if got.failures != 20 || len(got.warnings) != 20 || !sort.StringsAreSorted(got.warnings) {
		t.Fatalf("cancellation outcomes not deterministic: failures=%d warnings=%v", got.failures, got.warnings)
	}
}
