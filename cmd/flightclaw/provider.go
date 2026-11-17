package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/krisukox/google-flights-api/flights"
	"golang.org/x/text/currency"
	"golang.org/x/text/language"
)

type LegQuery struct {
	Origins, Destinations []string
	Date                  string
	Adults                int
	Direct                bool
}

type FlightProvider interface {
	SearchOneWay(context.Context, LegQuery) ([]Journey, []string, error)
}

type googleFlightsProvider struct{ client *http.Client }

func newGoogleFlightsProvider() (FlightProvider, error) {
	return &googleFlightsProvider{client: &http.Client{}}, nil
}

func flightArgs(query LegQuery) (flights.Args, error) {
	date, err := time.Parse(time.DateOnly, query.Date)
	if err != nil {
		return flights.Args{}, err
	}
	stops := flights.AnyStops
	if query.Direct {
		stops = flights.Nonstop
	}
	return flights.Args{
		Date: date, ReturnDate: date,
		SrcAirports: query.Origins, DstAirports: query.Destinations,
		Options: flights.Options{Travelers: flights.Travelers{Adults: query.Adults}, Currency: currency.GBP, Stops: stops, Class: flights.Economy, TripType: flights.OneWay, Lang: language.English},
	}, nil
}

func (p *googleFlightsProvider) SearchOneWay(ctx context.Context, query LegQuery) ([]Journey, []string, error) {
	args, err := flightArgs(query)
	if err != nil {
		return nil, nil, err
	}
	session := &flights.Session{}
	searchURL, err := session.SerializeURL(ctx, args)
	if err != nil {
		return nil, nil, err
	}
	u, err := url.Parse(searchURL)
	if err != nil {
		return nil, nil, err
	}
	u.Path = "/travel/flights"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch Google Flights search page: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("Google Flights search page returned HTTP %d", response.StatusCode)
	}
	const maxPageSize = 16 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageSize+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read Google Flights search page: %w", err)
	}
	if len(body) > maxPageSize {
		return nil, nil, fmt.Errorf("Google Flights search page exceeds %d bytes", maxPageSize)
	}
	offers, warnings, err := parseGooglePage(body)
	if err != nil {
		return nil, nil, err
	}
	journeys, conversionWarnings := convertOffers(offers, u.String())
	return journeys, append(warnings, conversionWarnings...), nil
}

func convertOffers(offers []flights.FullOffer, bookingURL string) ([]Journey, []string) {
	journeys := make([]Journey, 0, len(offers))
	warnings := []string{}
	for i, offer := range offers {
		if len(offer.Flight) == 0 {
			warnings = append(warnings, fmt.Sprintf("offer %d ignored: no flight segments", i+1))
			continue
		}
		segments := make([]Segment, 0, len(offer.Flight))
		for _, flight := range offer.Flight {
			segments = append(segments, Segment{
				Airline: flight.AirlineName, FlightNumber: flight.FlightNumber,
				DepartureAirport: flight.DepAirportCode, ArrivalAirport: flight.ArrAirportCode,
				DepartureTime: flight.DepTime, ArrivalTime: flight.ArrTime,
				DurationMinutes: int(flight.Duration / time.Minute),
			})
		}
		duration := offer.FlightDuration
		if duration <= 0 {
			duration = segments[len(segments)-1].ArrivalTime.Sub(segments[0].DepartureTime)
		}
		date := offer.StartDate.Format(time.DateOnly)
		if offer.StartDate.IsZero() {
			date = segments[0].DepartureTime.Format(time.DateOnly)
		}
		journeys = append(journeys, Journey{
			Date: date, Price: offer.Price, Currency: "GBP", DurationMinutes: int(duration / time.Minute),
			Stops: len(segments) - 1, Segments: segments, BookingURL: bookingURL,
		})
	}
	return journeys, warnings
}
