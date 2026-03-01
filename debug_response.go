// +build ignore

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/krisukox/google-flights-api/flights"
	"golang.org/x/text/language"
)

func main() {
	session, _ := flights.New()
	ctx := context.Background()

	depDate, _ := time.Parse("2006-01-02", "2026-03-20")
	retDate, _ := time.Parse("2006-01-02", "2026-03-23")

	args := flights.Args{
		Date:        depDate,
		ReturnDate:  retDate,
		SrcAirports: []string{"LGW"},
		DstAirports: []string{"FNC"},
		Options: flights.Options{
			Travelers: flights.Travelers{Adults: 1},
			Class:     flights.Economy,
			Stops:     flights.AnyStops,
			Lang:      language.English,
			TripType:  flights.RoundTrip,
		},
	}

	// We need to make the raw request ourselves since the library doesn't expose it
	// Let's use the library's GetOffers but also dump intermediate data
	offers, _, err := session.GetOffers(ctx, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print what we got
	for i, o := range offers {
		if i >= 3 {
			break
		}
		fmt.Printf("Offer %d: £%.0f\n", i+1, o.Price)
		fmt.Printf("  Outbound: %d flights\n", len(o.Flight))
		for _, f := range o.Flight {
			fmt.Printf("    %s %s: %s %s → %s %s\n", f.AirlineName, f.FlightNumber, f.DepAirportCode, f.DepTime.Format("15:04"), f.ArrAirportCode, f.ArrTime.Format("15:04"))
		}
		fmt.Printf("  Return: %d flights\n", len(o.ReturnFlight))
	}

	_ = ctx
	_ = args
	_ = http.MethodPost
	_ = url.QueryEscape
	_ = retryablehttp.NewRequest
	_ = bufio.NewReader
	_ = bytes.NewReader
	_ = json.Unmarshal
	_ = strconv.FormatInt
}
