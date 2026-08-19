package osgrid

import (
	"math"
	"testing"
)

// TestInverseCaister checks the inverse Transverse Mercator against OS's own
// worked example (Caister water tower): E 651409.903, N 313177.270 should map
// back to the Airy 1830 (OSGB36) lat/lon 52.6575703, 1.7179216 — the exact
// inverse of the airyToEN worked example — to sub-metre accuracy. (Note these
// are OSGB36 values; ToWGS84 then applies the datum shift on top.)
func TestInverseCaister(t *testing.T) {
	latR, lonR := enToAiry(651409.903, 313177.270)
	lat, lon := latR*180/math.Pi, lonR*180/math.Pi
	if math.Abs(lat-52.6575703) > 1e-6 || math.Abs(lon-1.7179216) > 1e-6 {
		t.Errorf("enToAiry Caister = %.7f, %.7f; want 52.6575703, 1.7179216", lat, lon)
	}
}

// TestRoundTrip converts several WGS84 points to the grid and back; the round
// trip should return the original to well under a metre.
func TestRoundTrip(t *testing.T) {
	points := []struct {
		name     string
		lat, lon float64
	}{
		{"Tower of London", 51.508039, -0.076006},
		{"Ben Nevis", 56.796849, -5.003508},
		{"Cardiff Castle", 51.482300, -3.181000},
		{"Land's End", 50.066360, -5.714680},
	}
	for _, p := range points {
		ref, ok := FromWGS84(p.lat, p.lon)
		if !ok {
			t.Errorf("%s: FromWGS84 returned not-ok", p.name)
			continue
		}
		lat, lon, ok := ToWGS84(float64(ref.Easting), float64(ref.Northing))
		if !ok {
			t.Errorf("%s: ToWGS84 returned not-ok", p.name)
			continue
		}
		// Easting/Northing were rounded to whole metres, so allow ~2e-5 deg.
		if math.Abs(lat-p.lat) > 2e-5 || math.Abs(lon-p.lon) > 3e-5 {
			t.Errorf("%s round-trip: got %.6f, %.6f; want %.6f, %.6f", p.name, lat, lon, p.lat, p.lon)
		}
	}
}

func TestToWGS84OutsideGrid(t *testing.T) {
	if _, _, ok := ToWGS84(-1, 500000); ok {
		t.Error("negative easting should be outside the grid")
	}
	if _, _, ok := ToWGS84(500000, 2000000); ok {
		t.Error("huge northing should be outside the grid")
	}
}
