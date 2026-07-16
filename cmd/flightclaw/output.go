package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

func formatDuration(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
}

func renderJourney(w io.Writer, heading string, journey Journey) error {
	stopLabel := "stops"
	if journey.Stops == 1 {
		stopLabel = "stop"
	}
	if _, err := fmt.Fprintf(w, "  %s — %s, %d %s\n", heading, formatDuration(journey.DurationMinutes), journey.Stops, stopLabel); err != nil {
		return err
	}
	for _, segment := range journey.Segments {
		if _, err := fmt.Fprintf(w, "    %s %s: %s → %s (%s)\n",
			segment.Airline, segment.FlightNumber,
			formatEndpoint(segment.DepartureAirport, segment.DepartureTime),
			formatEndpoint(segment.ArrivalAirport, segment.ArrivalTime),
			formatDuration(segment.DurationMinutes)); err != nil {
			return err
		}
	}
	if journey.BookingURL != "" {
		if _, err := fmt.Fprintf(w, "    Booking: %s\n", journey.BookingURL); err != nil {
			return err
		}
	}
	return nil
}

func formatEndpoint(airport string, at time.Time) string {
	return fmt.Sprintf("%s %s", airport, at.Format("2006-01-02 15:04 -07:00"))
}

func renderHuman(w io.Writer, response SearchResponse) error {
	for i, itinerary := range response.Results {
		currency := itinerary.Currency
		if currency == "GBP" || currency == "" {
			currency = "£"
		} else {
			currency += " "
		}
		if _, err := fmt.Fprintf(w, "Option %d: %s%.2f\n", i+1, currency, itinerary.TotalPrice); err != nil {
			return err
		}
		if err := renderJourney(w, "Outbound", itinerary.Outbound); err != nil {
			return err
		}
		if itinerary.Return != nil {
			if err := renderJourney(w, "Return", *itinerary.Return); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if len(response.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "Warnings:"); err != nil {
			return err
		}
		for _, warning := range response.Warnings {
			if _, err := fmt.Fprintf(w, "  - %s\n", warning); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderJSON(w io.Writer, response SearchResponse) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}
