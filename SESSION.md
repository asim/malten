## Last Session
- Added live bus arrivals via TfL API
- /bus command shows nearby stops and real-time arrivals
- Local context auto-fetches when location enabled (see bus times on page load)
- Speech-to-text input
- Service worker for offline PWA

## Next Up
- Add train times (National Rail API / RTT)
- Events API (Eventbrite, local events)
- Extend context to show more: weather? prayer times?
- Consider caching TfL responses briefly (30s?)

## Key Decisions
- TfL API is free, no auth needed, UK coverage (buses/tubes)
- Context shown automatically when location enabled
- Bus arrivals grouped by stop name, limited to 3 stops

## Live Data Sources
- TfL Unified API: buses, tubes (implemented)
- National Rail Darwin: trains (needs registration)
- Bus Open Data Service: UK-wide buses
- Eventbrite: local events
