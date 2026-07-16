package main

import "time"

type Segment struct {
	Airline          string    `json:"airline"`
	FlightNumber     string    `json:"flight_number"`
	DepartureAirport string    `json:"departure_airport"`
	ArrivalAirport   string    `json:"arrival_airport"`
	DepartureTime    time.Time `json:"departure_time"`
	ArrivalTime      time.Time `json:"arrival_time"`
	DurationMinutes  int       `json:"duration_minutes"`
}

type Journey struct {
	Date            string    `json:"date"`
	Price           float64   `json:"price"`
	Currency        string    `json:"currency"`
	DurationMinutes int       `json:"duration_minutes"`
	Stops           int       `json:"stops"`
	Segments        []Segment `json:"segments"`
	BookingURL      string    `json:"booking_url,omitempty"`
}

type Itinerary struct {
	TotalPrice float64  `json:"total_price"`
	Currency   string   `json:"currency"`
	Outbound   Journey  `json:"outbound"`
	Return     *Journey `json:"return,omitempty"`
}

type QuerySummary struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Dates          []string `json:"dates,omitempty"`
	ReturnFrom     string   `json:"return_from,omitempty"`
	ReturnTo       string   `json:"return_to,omitempty"`
	ReturnDates    []string `json:"return_dates,omitempty"`
	Adults         int      `json:"adults"`
	Direct         bool     `json:"direct"`
	CandidateCount int      `json:"candidate_count"`
}

type SearchResponse struct {
	Query    QuerySummary `json:"query"`
	Results  []Itinerary  `json:"results"`
	Warnings []string     `json:"warnings,omitempty"`
}
