package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

var cityAliases = map[string][]string{
	"LON": {"LHR", "LGW", "STN", "LTN", "LCY", "SEN"},
	"NYC": {"JFK", "LGA", "EWR"},
	"PAR": {"CDG", "ORY", "BVA"},
	"ROM": {"FCO", "CIA"},
	"MIL": {"MXP", "LIN", "BGY"},
	"TYO": {"HND", "NRT"},
	"OSA": {"KIX", "ITM", "UKB"},
	"SEL": {"ICN", "GMP"},
	"BJS": {"PEK", "PKX"},
	"SHA": {"PVG", "SHA"},
	"CHI": {"ORD", "MDW"},
	"WAS": {"IAD", "DCA", "BWI"},
	"BUE": {"EZE", "AEP"},
}

func normalizeCode(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 3 {
		return "", fmt.Errorf("invalid airport/city code %q: must be exactly three ASCII letters", code)
	}
	for i := 0; i < len(code); i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return "", fmt.Errorf("invalid airport/city code %q: must be exactly three ASCII letters", code)
		}
	}
	return code, nil
}

func expandLocation(code string) ([]string, error) {
	normalized, err := normalizeCode(code)
	if err != nil {
		return nil, err
	}
	if airports, ok := cityAliases[normalized]; ok {
		return append([]string(nil), airports...), nil
	}
	return []string{normalized}, nil
}

type queryOutcome struct {
	query    LegQuery
	journeys []Journey
	warnings []string
	err      error
}

const searchWorkerCount = 4

func searchLegs(ctx context.Context, provider FlightProvider, queries []LegQuery) ([]Journey, []string, int) {
	if len(queries) == 0 {
		return nil, nil, 0
	}
	jobs := make(chan LegQuery, len(queries))
	outcomes := make(chan queryOutcome, len(queries))
	for _, query := range queries {
		jobs <- query
	}
	close(jobs)
	workers := searchWorkerCount
	if len(queries) < workers {
		workers = len(queries)
	}
	for i := 0; i < workers; i++ {
		go func() {
			for query := range jobs {
				select {
				case <-ctx.Done():
					outcomes <- queryOutcome{query: query, err: ctx.Err()}
					continue
				default:
				}
				journeys, warnings, err := provider.SearchOneWay(ctx, query)
				outcomes <- queryOutcome{query: query, journeys: journeys, warnings: warnings, err: err}
			}
		}()
	}
	var journeys []Journey
	var warnings []string
	failures := 0
	for range queries {
		outcome := <-outcomes
		if outcome.err != nil {
			failures++
			warnings = append(warnings, fmt.Sprintf("%s → %s on %s failed: %v", strings.Join(outcome.query.Origins, "/"), strings.Join(outcome.query.Destinations, "/"), outcome.query.Date, outcome.err))
			continue
		}
		journeys = append(journeys, outcome.journeys...)
		for _, warning := range outcome.warnings {
			warnings = append(warnings, fmt.Sprintf("%s → %s on %s: %s", strings.Join(outcome.query.Origins, "/"), strings.Join(outcome.query.Destinations, "/"), outcome.query.Date, warning))
		}
	}
	sort.Strings(warnings)
	return journeys, warnings, failures
}

func journeyMatches(journey Journey, direct bool, maxDuration time.Duration, filter TimeFilter) bool {
	if len(journey.Segments) == 0 {
		return false
	}
	if direct && (journey.Stops != 0 || len(journey.Segments) != 1) {
		return false
	}
	if maxDuration > 0 && time.Duration(journey.DurationMinutes)*time.Minute > maxDuration {
		return false
	}
	departure := journey.Segments[0].DepartureTime
	arrival := journey.Segments[len(journey.Segments)-1].ArrivalTime
	departureMinutes := departure.Hour()*60 + departure.Minute()
	arrivalMinutes := arrival.Hour()*60 + arrival.Minute()
	if filter.DepartAfter >= 0 && departureMinutes < filter.DepartAfter {
		return false
	}
	if filter.ArriveAfter >= 0 && arrivalMinutes < filter.ArriveAfter {
		return false
	}
	if filter.ArriveBefore >= 0 && arrivalMinutes > filter.ArriveBefore {
		return false
	}
	return true
}

func filterJourneys(journeys []Journey, direct bool, maxDuration time.Duration, filter TimeFilter) []Journey {
	filtered := journeys[:0]
	for _, journey := range journeys {
		if journeyMatches(journey, direct, maxDuration, filter) {
			filtered = append(filtered, journey)
		}
	}
	return filtered
}

func journeyKey(journey Journey) string {
	var key strings.Builder
	key.WriteString(strconv.FormatUint(math.Float64bits(journey.Price), 16))
	for _, segment := range journey.Segments {
		fmt.Fprintf(&key, "\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", segment.Airline, segment.FlightNumber,
			segment.DepartureAirport, segment.ArrivalAirport, segment.DepartureTime.Format(time.RFC3339Nano), segment.ArrivalTime.Format(time.RFC3339Nano))
	}
	return key.String()
}

func deduplicateJourneys(journeys []Journey) []Journey {
	seen := make(map[string]struct{}, len(journeys))
	unique := make([]Journey, 0, len(journeys))
	for _, journey := range journeys {
		key := journeyKey(journey)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, journey)
	}
	return unique
}

func searchFlights(parent context.Context, provider FlightProvider, cfg Config) (SearchResponse, error) {
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()
	origins, err := expandLocation(cfg.From)
	if err != nil {
		return SearchResponse{}, err
	}
	destinations, err := expandLocation(cfg.To)
	if err != nil {
		return SearchResponse{}, err
	}
	queries := make([]LegQuery, 0, len(cfg.Dates))
	for _, date := range cfg.Dates {
		queries = append(queries, LegQuery{Origins: origins, Destinations: destinations, Date: date, Adults: cfg.Adults, Direct: cfg.Direct})
	}
	journeys, warnings, failures := searchLegs(ctx, provider, queries)
	journeys = filterJourneys(journeys, cfg.Direct, cfg.MaxDuration, cfg.Outbound)
	journeys = deduplicateJourneys(journeys)
	if len(journeys) == 0 {
		if failures > 0 {
			return SearchResponse{}, fmt.Errorf("all outbound flight searches failed: %s", strings.Join(warnings, "; "))
		}
		return SearchResponse{}, fmt.Errorf("no outbound flights matched the search")
	}
	itineraries := make([]Itinerary, 0, len(journeys))
	if len(cfg.ReturnDates) == 0 {
		for _, journey := range journeys {
			itineraries = append(itineraries, Itinerary{TotalPrice: journey.Price, Currency: journey.Currency, Outbound: journey})
		}
	} else {
		returnOrigins, expandErr := expandLocation(cfg.ReturnFrom)
		if expandErr != nil {
			return SearchResponse{}, expandErr
		}
		returnDestinations, expandErr := expandLocation(cfg.ReturnTo)
		if expandErr != nil {
			return SearchResponse{}, expandErr
		}
		returnQueries := make([]LegQuery, 0, len(cfg.ReturnDates))
		for _, date := range cfg.ReturnDates {
			returnQueries = append(returnQueries, LegQuery{Origins: returnOrigins, Destinations: returnDestinations, Date: date, Adults: cfg.Adults, Direct: cfg.Direct})
		}
		returns, returnWarnings, returnFailures := searchLegs(ctx, provider, returnQueries)
		returns = filterJourneys(returns, cfg.Direct, cfg.MaxDuration, cfg.Return)
		returns = deduplicateJourneys(returns)
		warnings = append(warnings, returnWarnings...)
		sort.Strings(warnings)
		if len(returns) == 0 {
			if returnFailures > 0 {
				return SearchResponse{}, fmt.Errorf("all return flight searches failed: %s", strings.Join(returnWarnings, "; "))
			}
			return SearchResponse{}, fmt.Errorf("no return flights matched the search")
		}
		for _, outbound := range journeys {
			for _, back := range returns {
				if back.Date < outbound.Date {
					continue
				}
				backCopy := back
				currencyCode := outbound.Currency
				if currencyCode == "" {
					currencyCode = back.Currency
				}
				itineraries = append(itineraries, Itinerary{TotalPrice: outbound.Price + back.Price, Currency: currencyCode, Outbound: outbound, Return: &backCopy})
			}
		}
	}
	if len(itineraries) == 0 {
		return SearchResponse{}, fmt.Errorf("no valid outbound/return date combinations")
	}
	sort.SliceStable(itineraries, func(i, j int) bool {
		left, right := itineraries[i], itineraries[j]
		if left.TotalPrice != right.TotalPrice {
			return left.TotalPrice < right.TotalPrice
		}
		leftDeparture := left.Outbound.Segments[0].DepartureTime
		rightDeparture := right.Outbound.Segments[0].DepartureTime
		if !leftDeparture.Equal(rightDeparture) {
			return leftDeparture.Before(rightDeparture)
		}
		if left.Return != nil && right.Return != nil {
			leftReturn := left.Return.Segments[0].DepartureTime
			rightReturn := right.Return.Segments[0].DepartureTime
			if !leftReturn.Equal(rightReturn) {
				return leftReturn.Before(rightReturn)
			}
		}
		leftKey, rightKey := journeyKey(left.Outbound), journeyKey(right.Outbound)
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		if left.Return == nil || right.Return == nil {
			return left.Return == nil && right.Return != nil
		}
		return journeyKey(*left.Return) < journeyKey(*right.Return)
	})
	candidates := len(itineraries)
	if len(itineraries) > cfg.Results {
		itineraries = itineraries[:cfg.Results]
	}
	return SearchResponse{Query: QuerySummary{From: cfg.From, To: cfg.To, Dates: cfg.Dates, ReturnFrom: func() string {
		if len(cfg.ReturnDates) > 0 {
			return cfg.ReturnFrom
		}
		return ""
	}(), ReturnTo: func() string {
		if len(cfg.ReturnDates) > 0 {
			return cfg.ReturnTo
		}
		return ""
	}(), ReturnDates: cfg.ReturnDates, Adults: cfg.Adults, Direct: cfg.Direct, CandidateCount: candidates}, Results: itineraries, Warnings: warnings}, nil
}
