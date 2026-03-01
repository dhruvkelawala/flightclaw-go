package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type FlightResult struct {
	Price    float64     `json:"price"`
	Duration int         `json:"duration"`
	Stops    int         `json:"stops"`
	Legs     []FlightLeg `json:"legs"`
}

type FlightLeg struct {
	Airline        string `json:"airline"`
	FlightNumber   string `json:"flight_number"`
	Departure      string `json:"departure_airport"`
	Arrival        string `json:"arrival_airport"`
	DepartureTime  string `json:"departure_datetime"`
	ArrivalTime    string `json:"arrival_datetime"`
}

type SearchResult struct {
	Outbound FlightResult `json:"outbound"`
	Return   FlightResult `json:"return,omitempty"`
	Total    float64     `json:"total"`
}

func runPythonSearch(orig, dest, departDate, returnDate string, adults int) ([]SearchResult, error) {
	script := fmt.Sprintf(`
import json
from fli.search.flights import SearchFlights
from fli.models import FlightSearchFilters, FlightResult as FR, FlightLeg as FL, Airport, Airline
from fli.models.google_flights.base import FlightSegment, PassengerInfo, TripType, MaxStops, SeatType
from datetime import datetime

s = SearchFlights()
filters = FlightSearchFilters(
    trip_type=TripType.ROUND_TRIP if '%s' else TripType.ONE_WAY,
    passenger_info=PassengerInfo(adults=%d),
    flight_segments=[
        FlightSegment(
            departure_airport=[(Airport.%s, 0)],
            arrival_airport=[(Airport.%s, 0)],
            travel_date='%s',
        ),
    ] + ([
        FlightSegment(
            departure_airport=[(Airport.%s, 0)],
            arrival_airport=[(Airport.%s, 0)],
            travel_date='%s',
        ),
    ] if '%s' else []),
    stops=MaxStops.ANY,
    seat_type=SeatType.ECONOMY,
)

results = s.search(filters, top_n=5)
out = []
if results:
    for r in results:
        if isinstance(r, tuple):
            out.append({
                "outbound": {"price": r[0].price, "legs": [{"airline": l.airline.name, "flight_number": l.flight_number, "departure_airport": l.departure_airport.name, "arrival_airport": l.arrival_airport.name, "departure_datetime": str(l.departure_datetime), "arrival_datetime": str(l.arrival_datetime)} for l in r[0].legs]},
                "return": {"price": r[1].price, "legs": [{"airline": l.airline.name, "flight_number": l.flight_number, "departure_airport": l.departure_airport.name, "arrival_airport": l.arrival_airport.name, "departure_datetime": str(l.departure_datetime), "arrival_datetime": str(l.arrival_datetime)} for l in r[1].legs]},
                "total": r[0].price + r[1].price,
            })
        else:
            out.append({
                "outbound": {"price": r.price, "legs": [{"airline": l.airline.name, "flight_number": l.flight_number, "departure_airport": l.departure_airport.name, "arrival_airport": l.arrival_airport.name, "departure_datetime": str(l.departure_datetime), "arrival_datetime": str(l.arrival_datetime)} for l in r.legs]},
                "total": r.price,
            })
print(json.dumps(out))
`, returnDate, adults, orig, dest, departDate, dest, orig, returnDate, returnDate)

	cmd := exec.Command("python3", "-c", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("python error: %v %s", err, stderr.String())
	}
	
	var results []SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil, fmt.Errorf("parse error: %v, output: %s", err, stdout.String())
	}
	return results, nil
}

func main() {
	originPtr := flag.String("from", "LGW", "Departure airport")
	destPtr := flag.String("to", "FNC", "Destination airport")
	departure := flag.String("date", "", "Departure date (YYYY-MM-DD)")
	return_ := flag.String("return", "", "Return date (YYYY-MM-DD)")
	results := flag.Int("results", 5, "Number of results")
	adults := flag.Int("adults", 1, "Number of adults")
	flag.Parse()

	orig := strings.ToUpper(*originPtr)
	dest := strings.ToUpper(*destPtr)
	
	if *departure == "" {
		fmt.Println("Error: --date is required")
		return
	}
	
	depDate, err := time.Parse("2006-01-02", *departure)
	if err != nil {
		fmt.Printf("Error parsing date: %v\n", err)
		return
	}
	
	fmt.Printf("Searching %s → %s on %s", orig, dest, depDate.Format("02 Jan 2006"))
	if *return_ != "" {
		retDate, _ := time.Parse("2006-01-02", *return_)
		fmt.Printf(" (return %s)\n", retDate.Format("02 Jan 2006"))
	} else {
		fmt.Println()
	}
	fmt.Println(strings.Repeat("─", 60))
	
	searchResults, err := runPythonSearch(orig, dest, *departure, *return_, *adults)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	for i, r := range searchResults {
		if i >= *results {
			break
		}
		
		if len(r.Return.Legs) > 0 {
			fmt.Printf("\nOption %d: £%.0f round trip\n", i+1, r.Total)
			fmt.Printf("  Outbound: %s %s %s %s → %s\n", 
				r.Outbound.Legs[0].Airline, r.Outbound.Legs[0].FlightNumber,
				r.Outbound.Legs[0].Departure, r.Outbound.Legs[0].DepartureTime[11:16],
				r.Outbound.Legs[0].Arrival)
			fmt.Printf("  Return:   %s %s %s %s → %s\n", 
				r.Return.Legs[0].Airline, r.Return.Legs[0].FlightNumber,
				r.Return.Legs[0].Departure, r.Return.Legs[0].DepartureTime[11:16],
				r.Return.Legs[0].Arrival)
		} else {
			fmt.Printf("\nOption %d: £%.0f\n", i+1, r.Total)
			fmt.Printf("  %s %s %s → %s\n", 
				r.Outbound.Legs[0].Airline, r.Outbound.Legs[0].FlightNumber,
				r.Outbound.Legs[0].Departure, r.Outbound.Legs[0].Arrival)
		}
	}
	
	fmt.Printf("\n%d result(s)\n", len(searchResults))
	_ = strconv.FormatInt(time.Now().Unix(), 10)
}
