// Package nrail adds National Rail: nearby stations (from a vendored, embedded
// dataset) and live departure boards (Darwin OpenLDBWS). It gives the map and
// the "Ask Malten" agent live rail data nationwide, beyond London's TfL.
//
// The station coordinates are a vendored copy of davwheat/uk-railway-stations
// (Open Database License, ODbL) — CRS code, name and WGS84 lat/lng for ~2,600
// Great Britain stations. Kept embedded so the binary stays self-contained.
package nrail

import (
	"embed"
	"encoding/csv"
	"math"
	"sort"
	"strconv"
	"strings"
)

//go:embed stations.csv
var stationFS embed.FS

// Station is a National Rail station.
type Station struct {
	CRS  string  `json:"crs"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	// Miles is set on results from Nearest: the great-circle distance from the
	// query point, in miles.
	Miles float64 `json:"miles,omitempty"`
}

var stations []Station

func init() {
	f, err := stationFS.Open("stations.csv")
	if err != nil {
		return
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return
	}
	for i, row := range rows {
		if i == 0 || len(row) < 4 { // header
			continue
		}
		lat, err1 := strconv.ParseFloat(row[1], 64)
		lng, err2 := strconv.ParseFloat(row[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		stations = append(stations, Station{CRS: strings.ToUpper(row[0]), Name: row[3], Lat: lat, Lng: lng})
	}
}

// Count reports how many stations were loaded.
func Count() int { return len(stations) }

// Lookup returns the station with the given CRS code, if known.
func Lookup(crs string) (Station, bool) {
	crs = strings.ToUpper(strings.TrimSpace(crs))
	for _, s := range stations {
		if s.CRS == crs {
			return s, true
		}
	}
	return Station{}, false
}

// Nearest returns up to n stations closest to a lat/lng, nearest first, each
// carrying its distance in miles.
func Nearest(lat, lng float64, n int) []Station {
	if n <= 0 {
		n = 5
	}
	out := make([]Station, len(stations))
	for i, s := range stations {
		s.Miles = haversineMiles(lat, lng, s.Lat, s.Lng)
		out[i] = s
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Miles < out[j].Miles })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// haversineMiles returns the great-circle distance between two points in miles.
func haversineMiles(lat1, lng1, lat2, lng2 float64) float64 {
	const earthMiles = 3958.7613
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthMiles * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
