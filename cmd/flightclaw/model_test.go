package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSearchResponseJSONContractPreservesLocalOffset(t *testing.T) {
	when := time.Date(2026, 9, 13, 8, 30, 0, 0, time.FixedZone("BST", 3600))
	response := SearchResponse{
		Query: QuerySummary{From: "LTN", To: "FCO", CandidateCount: 1},
		Results: []Itinerary{{TotalPrice: 25, Currency: "GBP", Outbound: Journey{
			Date: "2026-09-13", Price: 25, Currency: "GBP", DurationMinutes: 150,
			Segments: []Segment{{Airline: "Example Air", FlightNumber: "EA25", DepartureAirport: "LTN", ArrivalAirport: "FCO", DepartureTime: when, ArrivalTime: when.Add(150 * time.Minute), DurationMinutes: 150}},
		}}},
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	results := got["results"].([]any)
	outbound := results[0].(map[string]any)["outbound"].(map[string]any)
	segment := outbound["segments"].([]any)[0].(map[string]any)
	if segment["departure_time"] != "2026-09-13T08:30:00+01:00" {
		t.Fatalf("offset lost: %v", segment["departure_time"])
	}
	if got["query"].(map[string]any)["candidate_count"] != float64(1) {
		t.Fatalf("query contract: %s", data)
	}
}
