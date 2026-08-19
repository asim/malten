package osgrid

import (
	"math"
	"testing"
)

// TestProjectionCaister validates the Transverse Mercator projection against the
// worked example in the OS "Guide to Coordinate Systems in Great Britain": the
// Caister water tower, OSGB36 52.657570°N 1.717922°E -> E 651409.9 N 313177.3.
func TestProjectionCaister(t *testing.T) {
	e, n := airyToEN(52.657570*math.Pi/180, 1.717922*math.Pi/180)
	if math.Abs(e-651409.903) > 0.2 || math.Abs(n-313177.270) > 0.2 {
		t.Fatalf("projection off: got E=%.3f N=%.3f, want E=651409.903 N=313177.270", e, n)
	}
	ref, ok := enToRef(e, n)
	if !ok || ref.GridRef != "TG 51409 13177" {
		t.Fatalf("grid ref: got %q (ok=%v), want \"TG 51409 13177\"", ref.GridRef, ok)
	}
}

// TestGridLettering checks the 100 km square lettering for a few known squares.
func TestGridLettering(t *testing.T) {
	cases := []struct {
		e, n    float64
		letters string
	}{
		{651409, 313177, "TG"}, // Caister
		{400000, 100000, "SU"}, // near the true origin
		{0, 0, "SV"},           // south-west origin square
		{529000, 179000, "TQ"}, // central London
	}
	for _, c := range cases {
		ref, ok := enToRef(c.e, c.n)
		if !ok || ref.GridRef[:2] != c.letters {
			t.Errorf("E=%.0f N=%.0f: got %q, want letters %s", c.e, c.n, ref.GridRef, c.letters)
		}
	}
}

// TestFromWGS84 checks the full pipeline lands in the right region: the Tower of
// London (WGS84) should give a TQ reference near easting 533xxx, northing 180xxx.
func TestFromWGS84(t *testing.T) {
	ref, ok := FromWGS84(51.5081, -0.0759)
	if !ok {
		t.Fatal("Tower of London should be inside the National Grid")
	}
	if ref.GridRef[:2] != "TQ" {
		t.Errorf("expected a TQ reference, got %q", ref.GridRef)
	}
	if math.Abs(float64(ref.Easting)-533500) > 800 || math.Abs(float64(ref.Northing)-180400) > 800 {
		t.Errorf("out of expected range: E=%d N=%d (want ~533500/180400)", ref.Easting, ref.Northing)
	}
}

// TestOutsideGB returns ok=false well outside Great Britain.
func TestOutsideGB(t *testing.T) {
	if _, ok := FromWGS84(48.8566, 2.3522); ok { // Paris
		t.Error("Paris should be outside the National Grid")
	}
}
