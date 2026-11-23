# Search page fixture

`search-page.html` contains three offers captured from the public Google Flights page on 17 November 2026, for one adult in economy, LHR to BOM on 11 February 2027, priced in GBP.

Source: https://www.google.com/travel/flights?hl=en&curr=GBP&q=one%20way%20LHR%20to%20BOM%20on%202027-02-11

The two retained top-flight offers are Gulf Air at £356 and Virgin Atlantic at £412. The retained other-flight offer is Royal Jordanian at £336. These are recorded prices for parser tests, not current quotes.

The fixture keeps the original segment arrays through the airline/flight-number field and the journey arrays through total duration. Unused offer fields, booking tokens and page metadata have been removed. It preserves omitted zero clock fields, connections, local dates and time zones, and both result groups.

`unsupported-page.html` preserves the `<head>` prefix (including `<link rel="canonical" href="https://www.google.com/travel/flights/unsupported">`) and visible upgrade message captured on 22 November 2026 when requesting Google Flights with Go's default `User-Agent` (`Go-http-client/2.0`), with inline scripts and stylesheets removed.
