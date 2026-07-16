package main

import (
	"context"
	"fmt"
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

type googleFlightsProvider struct{ session *flights.Session }

func newGoogleFlightsProvider() (FlightProvider, error) {
	session, err := flights.New()
	if err != nil {
		return nil, fmt.Errorf("initialize Google Flights session: %w", err)
	}
	return &googleFlightsProvider{session: session}, nil
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
	offers, _, err := p.session.GetOffers(ctx, args)
	if err != nil {
		return nil, nil, err
	}
	url, urlErr := p.session.SerializeURL(ctx, args)
	journeys, warnings := convertOffers(offers, url)
	if urlErr != nil {
		warnings = append(warnings, fmt.Sprintf("booking URL unavailable: %v", urlErr))
	}
	return journeys, warnings, nil
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
