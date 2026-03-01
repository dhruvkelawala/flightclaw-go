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

func validateAirportCode(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 3 {
		return fmt.Errorf("invalid airport code: %s (must be 3 letters)", code)
	}
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return fmt.Errorf("invalid airport code: %s (must be letters A-Z)", code)
		}
	}
	return nil
}

func runPythonSearch(orig, dest, departDate, returnDate string, adults int) ([]SearchResult, error) {
	// Validate inputs to prevent injection
	if err := validateAirportCode(orig); err != nil {
		return nil, fmt.Errorf("origin: %v", err)
	}
	if err := validateAirportCode(dest); err != nil {
		return nil, fmt.Errorf("destination: %v", err)
	}
	
	isRoundTrip := returnDate != ""
	
	// Build Python script as a static string (no user input interpolation)
	script := `
import json
import sys
from fli.search.flights import SearchFlights
from fli.models import FlightSearchFilters, FlightResult as FR, FlightLeg as FL, Airport, Airline
from fli.models.google_flights.base import FlightSegment, PassengerInfo, TripType, MaxStops, SeatType
from datetime import datetime

# Parse arguments
args = json.loads(sys.argv[1])
orig = args["origin"]
dest = args["dest"]
depart = args["depart"]
return_date = args["return"]
adults = args["adults"]
is_round_trip = args["is_round_trip"]

filters = FlightSearchFilters(
    trip_type=TripType.ROUND_TRIP if is_round_trip else TripType.ONE_WAY,
    passenger_info=PassengerInfo(adults=adults),
    flight_segments=[
        FlightSegment(
            departure_airport=[(Airport[orig], 0)],
            arrival_airport=[(Airport[dest], 0)],
            travel_date=depart,
        ),
    ] + ([
        FlightSegment(
            departure_airport=[(Airport[dest], 0)],
            arrival_airport=[(Airport[orig], 0)],
            travel_date=return_date,
        ),
    ] if is_round_trip else []),
    stops=MaxStops.ANY,
    seat_type=SeatType.ECONOMY,
)

s = SearchFlights()
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
`

	// Pass arguments via JSON to avoid any shell injection
	argsJSON, err := json.Marshal(map[string]interface{}{
		"origin":        orig,
		"dest":          dest,
		"depart":       departDate,
		"return":        returnDate,
		"adults":        adults,
		"is_round_trip": isRoundTrip,
	})
	if err != nil {
		return nil, fmt.Errorf("args marshal error: %v", err)
	}

	// Use args list - no shell injection possible
	cmd := exec.Command("python3", "-c", script, string(argsJSON))
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
	// Custom usage function
	flag.Usage = func() {
		fmt.Println("flightclaw - Google Flights CLI search tool")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  flightclaw [flags]")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  # Round-trip from London to Lisbon")
		fmt.Println("  flightclaw --from LGW --to FNC --date 2026-04-15 --return 2026-04-22")
		fmt.Println()
		fmt.Println("  # One-way flight")
		fmt.Println("  flightclaw --from LHR --to JFK --date 2026-05-01")
		fmt.Println()
		fmt.Println("  # Multiple results")
		fmt.Println("  flightclaw --from LGW --to FNC --date 2026-04-15 --return 2026-04-22 --results 10")
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
	}

	originPtr := flag.String("from", "LGW", "Departure airport (IATA code, e.g., LGW)")
	destPtr := flag.String("to", "FNC", "Destination airport (IATA code, e.g., FNC)")
	departure := flag.String("date", "", "Departure date (YYYY-MM-DD) [required]")
	return_ := flag.String("return", "", "Return date for round-trip (YYYY-MM-DD)")
	results := flag.Int("results", 5, "Number of results to display")
	adults := flag.Int("adults", 1, "Number of adult passengers")
	flag.Parse()

	// Show help if requested
	if flag.NFlag() == 0 || flag.Arg(0) == "-h" || flag.Arg(0) == "--help" {
		flag.Usage()
		return
	}

	orig := strings.ToUpper(*originPtr)
	dest := strings.ToUpper(*destPtr)
	
	if *departure == "" {
		fmt.Println("Error: --date is required")
		fmt.Println("Run with -h for usage examples")
		return
	}
	
	depDate, err := time.Parse("2006-01-02", *departure)
	if err != nil {
		fmt.Printf("Error parsing date: %v\n", err)
		return
	}
	
	// Validate airport codes
	if err := validateAirportCode(orig); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if err := validateAirportCode(dest); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("Searching %s → %s on %s", orig, dest, depDate.Format("02 Jan 2006"))
	if *return_ != "" {
		retDate, _ := time.Parse("2006-01-02", *return_)
		fmt.Printf(" (return %s)\n", retDate.Format("02 Jan 2006"))
	} else {
		fmt.Println(" (one-way)")
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
			fmt.Printf("  %s %s %s %s → %s\n", 
				r.Outbound.Legs[0].Airline, r.Outbound.Legs[0].FlightNumber,
				r.Outbound.Legs[0].Departure, r.Outbound.Legs[0].DepartureTime[11:16],
				r.Outbound.Legs[0].Arrival)
		}
	}
	
	fmt.Printf("\n%d result(s)\n", len(searchResults))
	_ = strconv.FormatInt(time.Now().Unix(), 10)
}
