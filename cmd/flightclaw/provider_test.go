package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/krisukox/google-flights-api/flights"
	"golang.org/x/text/currency"
)

type flightTransport func(*http.Request) (*http.Response, error)

func (f flightTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestGoogleFlightsSearchReadsPublicPage(t *testing.T) {
	page, err := os.ReadFile("testdata/search-page.html")
	if err != nil {
		t.Fatal(err)
	}
	provider := &googleFlightsProvider{client: &http.Client{Transport: flightTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Host != "www.google.com" || request.URL.Query().Get("tfs") == "" || request.URL.Query().Get("curr") != "GBP" {
			t.Errorf("not a public GBP search: %s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(page)))}, nil
	})}}
	journeys, warnings, err := provider.SearchOneWay(context.Background(), LegQuery{Origins: []string{"LHR"}, Destinations: []string{"BOM"}, Date: "2027-02-11", Adults: 1})
	if err != nil || len(warnings) != 0 || len(journeys) != 3 {
		t.Fatalf("want 3 real fares from both groups; got %d, warnings=%v, err=%v", len(journeys), warnings, err)
	}
	if journeys[0].Price != 356 || journeys[1].Price != 412 || journeys[2].Price != 336 {
		t.Fatalf("GBP prices = %v, %v, %v", journeys[0].Price, journeys[1].Price, journeys[2].Price)
	}
	if journeys[0].DurationMinutes != 850 || journeys[0].Stops != 1 || journeys[1].Stops != 0 || journeys[2].DurationMinutes != 1825 {
		t.Fatalf("duration/stops lost: %#v", journeys)
	}
	connection := journeys[0].Segments[1]
	if connection.FlightNumber != "GF 56" || connection.DepartureTime.Format(time.RFC3339) != "2027-02-11T23:00:00+03:00" || connection.ArrivalTime.Format(time.RFC3339) != "2027-02-12T05:10:00+05:30" {
		t.Fatalf("connection/local times lost: %#v", connection)
	}
	if journeys[2].Segments[0].ArrivalTime.Format(time.RFC3339) != "2027-02-12T00:05:00+03:00" || journeys[2].Segments[1].ArrivalTime.Format(time.RFC3339) != "2027-02-13T04:00:00+05:30" {
		t.Fatalf("omitted zero clock fields or overnight dates lost: %#v", journeys[2])
	}
}

func TestGoogleFlightsSearchReportsProviderFailures(t *testing.T) {
	page, err := os.ReadFile("testdata/search-page.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"rate limited", http.StatusTooManyRequests, "", "HTTP 429"},
		{"missing data", http.StatusOK, "<html>Consent required</html>", "missing flight data"},
		{"broken data", http.StatusOK, "AF_initDataCallback({key: 'ds:1', data:[broken], sideChannel: {}})", "invalid flight data"},
		{"changed group", http.StatusOK, "AF_initDataCallback({key: 'ds:1', data:[null,null,{},null], sideChannel: {}})", "unexpected format"},
		{"unpriced offers", http.StatusOK, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(page), "[[null,356]]", "[[null,null]]"), "[[null,412]]", "[[null,null]]"), "[[null,336]]", "[[null,null]]"), "none could be decoded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &googleFlightsProvider{client: &http.Client{Transport: flightTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}}
			journeys, _, err := provider.SearchOneWay(context.Background(), LegQuery{Origins: []string{"LHR"}, Destinations: []string{"BOM"}, Date: "2027-02-11", Adults: 1})
			if err == nil || !strings.Contains(err.Error(), tc.want) || len(journeys) != 0 {
				t.Fatalf("want error containing %q, got journeys=%v, err=%v", tc.want, journeys, err)
			}
		})
	}
}

func TestConvertOffersIncludesEverySegmentAndRejectsMalformedOffers(t *testing.T) {
	bst := time.FixedZone("BST", 3600)
	cest := time.FixedZone("CEST", 7200)
	offers := []flights.FullOffer{
		{Offer: flights.Offer{StartDate: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC), Price: 47}, FlightDuration: 4*time.Hour + 10*time.Minute, Flight: []flights.Flight{
			{AirlineName: "Example", FlightNumber: "EX1", DepAirportCode: "LTN", ArrAirportCode: "BCN", DepTime: time.Date(2026, 9, 13, 8, 0, 0, 0, bst), ArrTime: time.Date(2026, 9, 13, 11, 0, 0, 0, cest), Duration: 2 * time.Hour},
			{AirlineName: "Example", FlightNumber: "EX2", DepAirportCode: "BCN", ArrAirportCode: "FCO", DepTime: time.Date(2026, 9, 13, 12, 0, 0, 0, cest), ArrTime: time.Date(2026, 9, 13, 14, 10, 0, 0, cest), Duration: 2*time.Hour + 10*time.Minute},
		}},
		{Offer: flights.Offer{Price: 1}},
	}
	got, warnings := convertOffers(offers, "https://example.test/booking")
	if len(got) != 1 || len(warnings) != 1 {
		t.Fatalf("got %d journeys, warnings %v", len(got), warnings)
	}
	journey := got[0]
	if journey.Price != 47 || journey.Currency != "GBP" || journey.DurationMinutes != 250 || journey.Stops != 1 || len(journey.Segments) != 2 {
		t.Fatalf("incomplete journey: %#v", journey)
	}
	if journey.Segments[1].ArrivalAirport != "FCO" || journey.Segments[1].ArrivalTime.Format(time.RFC3339) != "2026-09-13T14:10:00+02:00" {
		t.Fatalf("last segment/local offset lost: %#v", journey.Segments[1])
	}
	if journey.BookingURL != "https://example.test/booking" {
		t.Fatalf("booking URL = %q", journey.BookingURL)
	}
}

func TestFlightArgsUsesOneWayGBPAdultsAndDirectStops(t *testing.T) {
	args, err := flightArgs(LegQuery{Origins: []string{"LHR", "LGW"}, Destinations: []string{"FCO", "CIA"}, Date: "2026-09-13", Adults: 2, Direct: true})
	if err != nil {
		t.Fatal(err)
	}
	if args.Options.TripType != flights.OneWay || args.Options.Currency != currency.GBP || args.Options.Travelers.Adults != 2 || args.Options.Stops != flights.Nonstop {
		t.Fatalf("provider options = %#v", args.Options)
	}
	if len(args.SrcAirports) != 2 || len(args.DstAirports) != 2 || args.Date.Format(time.DateOnly) != "2026-09-13" {
		t.Fatalf("provider args = %#v", args)
	}
}

func TestConvertOffersConvertsDirectJourney(t *testing.T) {
	departure := time.Date(2026, 9, 13, 8, 0, 0, 0, time.FixedZone("BST", 3600))
	offers := []flights.FullOffer{{Offer: flights.Offer{Price: 25}, FlightDuration: 2 * time.Hour, Flight: []flights.Flight{{
		AirlineName: "Direct Air", FlightNumber: "DA1", DepAirportCode: "LTN", ArrAirportCode: "FCO",
		DepTime: departure, ArrTime: departure.Add(2 * time.Hour), Duration: 2 * time.Hour,
	}}}}
	journeys, warnings := convertOffers(offers, "")
	if len(warnings) != 0 || len(journeys) != 1 || journeys[0].Stops != 0 || len(journeys[0].Segments) != 1 || journeys[0].Segments[0].ArrivalAirport != "FCO" {
		t.Fatalf("direct conversion = %#v, warnings=%v", journeys, warnings)
	}
}
