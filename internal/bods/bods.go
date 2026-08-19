// Package bods adds nationwide live buses via the Bus Open Data Service (BODS).
// It reads BODS's SIRI-VM "datafeed" (live vehicle positions) for a bounding
// box and normalizes it to a small set of moving vehicles for the map and the
// "Ask Malten" agent.
//
// SIRI-VM is XML, so this stays dependency-free (encoding/xml) — unlike the
// GTFS-RT protobuf feed, which would need an external library. A free BODS API
// key is required, held server-side as BODS_API_KEY (never sent to the browser).
package bods

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultDatafeedURL = "https://data.bus-data.dft.gov.uk/api/v1/datafeed/"

// Client calls the BODS SIRI-VM datafeed.
type Client struct {
	APIKey string
	URL    string
	HTTP   *http.Client
}

// New builds a client for the given BODS API key.
func New(apiKey string) *Client {
	return &Client{
		APIKey: strings.TrimSpace(apiKey),
		URL:    defaultDatafeedURL,
		HTTP:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Bus is a live vehicle position.
type Bus struct {
	Line        string  `json:"line"`
	Destination string  `json:"destination,omitempty"`
	Operator    string  `json:"operator,omitempty"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Bearing     float64 `json:"bearing,omitempty"`
}

// Vehicles returns live buses within the bounding box (minLng, minLat, maxLng,
// maxLat), capped at max. A cap of 0 means the built-in default.
func (c *Client) Vehicles(ctx context.Context, minLng, minLat, maxLng, maxLat float64, max int) ([]Bus, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("no BODS API key configured")
	}
	if max <= 0 || max > 400 {
		max = 250
	}
	base := c.URL
	if base == "" {
		base = defaultDatafeedURL
	}
	q := url.Values{}
	q.Set("api_key", c.APIKey)
	q.Set("boundingBox", fmt.Sprintf("%.5f,%.5f,%.5f,%.5f", minLng, minLat, maxLng, maxLat))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bods status %d", resp.StatusCode)
	}
	return parseVehicles(data, max)
}

// --- SIRI-VM parsing (local-name matching ignores the SIRI namespace) --------

type siriEnvelope struct {
	Activities []struct {
		Journey struct {
			LineRef           string `xml:"LineRef"`
			PublishedLineName string `xml:"PublishedLineName"`
			OperatorRef       string `xml:"OperatorRef"`
			DestinationName   string `xml:"DestinationName"`
			Bearing           string `xml:"Bearing"`
			Location          struct {
				Longitude string `xml:"Longitude"`
				Latitude  string `xml:"Latitude"`
			} `xml:"VehicleLocation"`
		} `xml:"MonitoredVehicleJourney"`
	} `xml:"ServiceDelivery>VehicleMonitoringDelivery>VehicleActivity"`
}

func parseVehicles(data []byte, max int) ([]Bus, error) {
	var env siriEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	out := make([]Bus, 0, len(env.Activities))
	for _, a := range env.Activities {
		j := a.Journey
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(j.Location.Latitude), 64)
		lng, err2 := strconv.ParseFloat(strings.TrimSpace(j.Location.Longitude), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		line := j.PublishedLineName
		if line == "" {
			line = j.LineRef
		}
		bearing, _ := strconv.ParseFloat(strings.TrimSpace(j.Bearing), 64)
		out = append(out, Bus{
			Line:        line,
			Destination: j.DestinationName,
			Operator:    j.OperatorRef,
			Lat:         lat,
			Lng:         lng,
			Bearing:     bearing,
		})
		if len(out) >= max {
			break
		}
	}
	return out, nil
}
