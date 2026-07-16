package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRenderHumanIncludesCompleteItineraryAndWarnings(t *testing.T) {
	bst := time.FixedZone("BST", 3600)
	cest := time.FixedZone("CEST", 7200)
	outbound := Journey{
		Date: "2026-09-13", Price: 47, Currency: "GBP", DurationMinutes: 310, Stops: 1, BookingURL: "https://example.test/book",
		Segments: []Segment{
			{Airline: "Example Air", FlightNumber: "EX1", DepartureAirport: "LTN", ArrivalAirport: "BCN", DepartureTime: time.Date(2026, 9, 13, 8, 0, 0, 0, bst), ArrivalTime: time.Date(2026, 9, 13, 11, 0, 0, 0, cest), DurationMinutes: 120},
			{Airline: "Example Air", FlightNumber: "EX2", DepartureAirport: "BCN", ArrivalAirport: "FCO", DepartureTime: time.Date(2026, 9, 13, 12, 0, 0, 0, cest), ArrivalTime: time.Date(2026, 9, 13, 14, 10, 0, 0, cest), DurationMinutes: 130},
		},
	}
	back := Journey{Date: "2026-09-26", Price: 30, Currency: "GBP", DurationMinutes: 180, Stops: 0, Segments: []Segment{
		{Airline: "Return Air", FlightNumber: "RT9", DepartureAirport: "NAP", ArrivalAirport: "LGW", DepartureTime: time.Date(2026, 9, 26, 18, 0, 0, 0, cest), ArrivalTime: time.Date(2026, 9, 26, 19, 0, 0, 0, bst), DurationMinutes: 180},
	}}
	response := SearchResponse{Results: []Itinerary{{TotalPrice: 77, Currency: "GBP", Outbound: outbound, Return: &back}}, Warnings: []string{"one date failed"}}

	var output bytes.Buffer
	if err := renderHuman(&output, response); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{
		"£77.00", "Outbound", "5h 10m", "1 stop", "Example Air EX1", "LTN 2026-09-13 08:00 +01:00 → BCN 2026-09-13 11:00 +02:00", "2h 0m",
		"Example Air EX2", "FCO 2026-09-13 14:10 +02:00", "Return", "Return Air RT9", "NAP 2026-09-26 18:00 +02:00 → LGW 2026-09-26 19:00 +01:00",
		"Booking: https://example.test/book", "Warnings:", "one date failed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderJSONIsStableStructuredJSONWithoutProse(t *testing.T) {
	response := SearchResponse{Query: QuerySummary{From: "LTN", To: "FCO", Adults: 1, CandidateCount: 1}, Results: []Itinerary{{TotalPrice: 25, Currency: "GBP", Outbound: testJourney("2026-09-13", "LTN", "FCO", 25, 8)}}, Warnings: []string{"partial warning"}}
	var output bytes.Buffer
	if err := renderJSON(&output, response); err != nil {
		t.Fatal(err)
	}
	var decoded SearchResponse
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON %q: %v", output.String(), err)
	}
	if decoded.Query.From != "LTN" || len(decoded.Results) != 1 || decoded.Results[0].Outbound.Segments[0].ArrivalAirport != "FCO" || decoded.Warnings[0] != "partial warning" {
		t.Fatalf("schema lost data: %#v", decoded)
	}
	if strings.Contains(output.String(), "Option") || strings.Contains(output.String(), "Warnings:") {
		t.Fatalf("JSON contaminated with prose: %s", output.String())
	}
}
