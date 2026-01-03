package spatial

// Region represents a geographic area with specific data sources
type Region struct {
	Name      string
	Transport []string // Available transport APIs
	// Bounding box
	MinLat, MaxLat float64
	MinLon, MaxLon float64
}

// Known regions with their data sources
var regions = []Region{
	{
		Name:      "london",
		Transport: []string{"tfl"},
		MinLat:    51.2, MaxLat: 51.7,
		MinLon:    -0.6, MaxLon: 0.3,
	},
	{
		Name:      "manchester",
		Transport: []string{"tfgm"}, // Transport for Greater Manchester (TODO)
		MinLat:    53.3, MaxLat: 53.6,
		MinLon:    -2.5, MaxLon: -2.0,
	},
	{
		Name:      "edinburgh",
		Transport: []string{"edinburgh_trams"}, // TODO
		MinLat:    55.85, MaxLat: 56.0,
		MinLon:    -3.4, MaxLon: -3.0,
	},
	{
		Name:      "cardiff",
		Transport: []string{"tfw"}, // Transport for Wales (TODO)
		MinLat:    51.4, MaxLat: 51.6,
		MinLon:    -3.3, MaxLon: -3.0,
	},
	{
		Name:      "dublin",
		Transport: []string{"dublin_bus", "irish_rail"}, // TODO
		MinLat:    53.2, MaxLat: 53.5,
		MinLon:    -6.5, MaxLon: -6.0,
	},
	// Add more as needed
}

// GetRegion returns the region for given coordinates, or nil if unknown
func GetRegion(lat, lon float64) *Region {
	for i := range regions {
		r := &regions[i]
		if lat >= r.MinLat && lat <= r.MaxLat && lon >= r.MinLon && lon <= r.MaxLon {
			return r
		}
	}
	return nil
}

// HasTransport checks if a region has a specific transport API
func (r *Region) HasTransport(api string) bool {
	if r == nil {
		return false
	}
	for _, t := range r.Transport {
		if t == api {
			return true
		}
	}
	return false
}

// IsLondon checks if coordinates are in London (TfL area)
func IsLondon(lat, lon float64) bool {
	r := GetRegion(lat, lon)
	return r != nil && r.Name == "london"
}

/*
Regional Data Sources Summary:

| Region     | Transport                    | Weather    | Prayer | POIs          |
|------------|------------------------------|------------|--------|---------------|
| London     | TfL ✓                        | Open-Meteo | Aladhan| OSM/Foursquare|
| Manchester | TfGM (TODO)                  | Open-Meteo | Aladhan| OSM/Foursquare|
| Edinburgh  | Edinburgh Trams (TODO)       | Open-Meteo | Aladhan| OSM/Foursquare|
| Cardiff    | Transport for Wales (TODO)   | Open-Meteo | Aladhan| OSM/Foursquare|
| Dublin     | Dublin Bus/Irish Rail (TODO) | Open-Meteo | Aladhan| OSM/Foursquare|
| Other UK   | National Rail (TODO)         | Open-Meteo | Aladhan| OSM/Foursquare|
| France     | SNCF (TODO)                  | Open-Meteo | Aladhan| OSM/Foursquare|
| USA        | Various (TODO)               | Open-Meteo | Aladhan| OSM/Foursquare|
| Global     | None                         | Open-Meteo | Aladhan| OSM/Foursquare|

APIs to integrate:
- TfGM: https://api.tfgm.com/ (Manchester buses/trams/rail)
- TfW: https://api.transport.wales/ (Wales buses/rail)
- Irish Rail: https://api.irishrail.ie/realtime/ (Ireland trains)
- Dublin Bus: https://data.dublinbus.ie/
- National Rail: https://opendata.nationalrail.co.uk/
- Edinburgh Trams: https://tfeapidocs.edinburgh.gov.uk/

For now: Only TfL is implemented. Other regions get weather/prayer/POIs but no live transport.
*/
