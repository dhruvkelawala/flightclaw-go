package main

import (
	"testing"
	"time"

	"github.com/krisukox/google-flights-api/flights"
	"golang.org/x/text/currency"
)

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
