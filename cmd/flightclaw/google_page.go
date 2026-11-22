package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"

	"github.com/krisukox/google-flights-api/flights"
	"github.com/krisukox/google-flights-api/iata"
)

const missingDataSnippetLimit = 400

var googlePageData = regexp.MustCompile(`AF_initDataCallback\(\{\s*key:\s*['"]ds:1['"]\s*,[^<]*?\bdata:\s*`)

func bodySnippet(body []byte) string {
	trimmed := bytes.TrimLeftFunc(body, unicode.IsSpace)
	if len(trimmed) == 0 {
		return "(empty)"
	}
	var out strings.Builder
	inSpace := false
	for len(trimmed) > 0 {
		r, size := utf8.DecodeRune(trimmed)
		trimmed = trimmed[size:]
		if unicode.IsSpace(r) {
			inSpace = true
			continue
		}
		if inSpace {
			if out.Len()+1 > missingDataSnippetLimit {
				out.WriteString("...")
				return out.String()
			}
			out.WriteByte(' ')
			inSpace = false
		}
		if out.Len()+utf8.RuneLen(r) > missingDataSnippetLimit {
			out.WriteString("...")
			return out.String()
		}
		out.WriteRune(r)
	}
	return out.String()
}

func parseGooglePage(body []byte) ([]flights.FullOffer, []string, error) {
	match := googlePageData.FindIndex(body)
	if match == nil {
		return nil, nil, fmt.Errorf("Google Flights search page is missing flight data (ds:1); response starts with: %q", bodySnippet(body))
	}
	var payload []json.RawMessage
	if err := json.NewDecoder(bytes.NewReader(body[match[1]:])).Decode(&payload); err != nil || len(payload) < 4 {
		return nil, nil, fmt.Errorf("Google Flights search page has invalid flight data (ds:1)")
	}
	var offers []flights.FullOffer
	var warnings []string
	rows := 0
	for _, group := range payload[2:4] {
		if bytes.Equal(bytes.TrimSpace(group), []byte("null")) {
			continue
		}
		var fields []json.RawMessage
		if err := json.Unmarshal(group, &fields); err != nil || len(fields) == 0 {
			return nil, nil, fmt.Errorf("Google Flights flight group has an unexpected format")
		}
		var rawOffers []json.RawMessage
		if err := json.Unmarshal(fields[0], &rawOffers); err != nil {
			return nil, nil, fmt.Errorf("Google Flights offers have an unexpected format: %w", err)
		}
		for _, rawOffer := range rawOffers {
			rows++
			offer, err := parseGoogleOffer(rawOffer)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("offer %d ignored: %v", rows, err))
				continue
			}
			offers = append(offers, offer)
		}
	}
	if rows > 0 && len(offers) == 0 {
		return nil, nil, fmt.Errorf("Google Flights returned %d offers but none could be decoded: %s", rows, warnings[0])
	}
	return offers, warnings, nil
}

func parseGoogleOffer(raw json.RawMessage) (flights.FullOffer, error) {
	var segments []json.RawMessage
	price := math.NaN()
	duration := 0
	details := []any{nil, nil, &segments, nil, nil, nil, nil, nil, nil, &duration}
	fields := []any{&details, &[]any{&[]any{nil, &price}}}
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) < 2 || len(details) < 10 || len(segments) == 0 || math.IsNaN(price) || math.IsInf(price, 0) || price <= 0 || duration <= 0 {
		return flights.FullOffer{}, fmt.Errorf("missing or invalid price, duration or segments")
	}
	offer := flights.FullOffer{Offer: flights.Offer{Price: price}, FlightDuration: time.Duration(duration) * time.Minute}
	for _, rawSegment := range segments {
		segment, err := parseGoogleSegment(rawSegment)
		if err != nil {
			return flights.FullOffer{}, err
		}
		offer.Flight = append(offer.Flight, segment)
	}
	offer.StartDate = offer.Flight[0].DepTime
	return offer, nil
}

func parseGoogleSegment(raw json.RawMessage) (flights.Flight, error) {
	var flight flights.Flight
	var departureDate, departureClock, arrivalDate, arrivalClock []int
	var carrier, number string
	duration := 0
	fields := []any{
		nil, nil, nil, &flight.DepAirportCode, nil, nil, &flight.ArrAirportCode, nil,
		&departureClock, nil, &arrivalClock, &duration, nil, nil, nil, nil, nil, nil, nil, nil,
		&departureDate, &arrivalDate, &[]any{&carrier, &number, nil, &flight.AirlineName},
	}
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) < 23 || flight.AirlineName == "" || carrier == "" || number == "" || duration <= 0 {
		return flight, fmt.Errorf("missing or invalid flight segment")
	}
	var err error
	flight.DepTime, err = googleLocalTime(flight.DepAirportCode, departureDate, departureClock)
	if err != nil {
		return flight, err
	}
	flight.ArrTime, err = googleLocalTime(flight.ArrAirportCode, arrivalDate, arrivalClock)
	if err != nil {
		return flight, err
	}
	if !flight.ArrTime.After(flight.DepTime) {
		return flight, fmt.Errorf("flight arrives before it departs")
	}
	flight.Duration = time.Duration(duration) * time.Minute
	flight.FlightNumber = carrier + " " + number
	return flight, nil
}

func googleLocalTime(airport string, date, clock []int) (time.Time, error) {
	if len(date) != 3 || len(clock) < 1 || len(clock) > 2 || date[0] < 2000 || date[1] < 1 || date[1] > 12 || date[2] < 1 || date[2] > 31 || clock[0] < 0 || clock[0] > 23 {
		return time.Time{}, fmt.Errorf("invalid local date/time for %s", airport)
	}
	minute := 0
	if len(clock) == 2 {
		minute = clock[1]
	}
	if minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("invalid local minute for %s", airport)
	}
	zone := iata.IATATimeZone(airport).Tz
	if zone == "" {
		return time.Time{}, fmt.Errorf("unknown airport time zone for %s", airport)
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, err
	}
	value := time.Date(date[0], time.Month(date[1]), date[2], clock[0], minute, 0, 0, location)
	if value.Day() != date[2] {
		return time.Time{}, fmt.Errorf("invalid local date for %s", airport)
	}
	return value, nil
}
