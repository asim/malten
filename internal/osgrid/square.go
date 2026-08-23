package osgrid

import "math"

// square.go works with the 1 km squares of the National Grid — the squares
// printed on every OS map ("TQ 15 68"). They make a good unit of exploration:
// real, named, the same for everyone, and roughly a walk across.
//
// Malten uses them for "new ground": the squares you've actually stood in. The
// browser remembers which; this side only does the geometry.

const squareM = 1000.0

// Neighbour is one of the squares around a point, with the centre of the square
// so it can be walked to or drawn.
type Neighbour struct {
	Square string  `json:"square"` // e.g. "TQ 15 69"
	Dir    string  `json:"dir"`    // "north", "north-east", …
	Lat    float64 `json:"lat"`    // centre of the square
	Lng    float64 `json:"lng"`
}

// compass, in the order the offsets below are walked.
var offsets = []struct {
	dx, dy int
	dir    string
}{
	{0, 1, "north"}, {1, 1, "north-east"}, {1, 0, "east"}, {1, -1, "south-east"},
	{0, -1, "south"}, {-1, -1, "south-west"}, {-1, 0, "west"}, {-1, 1, "north-west"},
}

// Neighbours returns the eight 1 km squares around a point, nearest-first by
// compass order. Squares that fall outside the National Grid (out to sea, or
// past the edge of Great Britain) are left out.
func Neighbours(lat, lon float64) []Neighbour {
	latR, lonR := wgs84ToAiry(lat*math.Pi/180, lon*math.Pi/180)
	e, n := airyToEN(latR, lonR)
	if _, ok := enToRef(e, n); !ok {
		return nil
	}
	// The south-west corner of the square the point is in.
	e0 := math.Floor(e/squareM) * squareM
	n0 := math.Floor(n/squareM) * squareM

	out := make([]Neighbour, 0, len(offsets))
	for _, o := range offsets {
		ce := e0 + float64(o.dx)*squareM + squareM/2
		cn := n0 + float64(o.dy)*squareM + squareM/2
		ref, ok := enToRef(ce, cn)
		if !ok {
			continue
		}
		clat, clng, ok := ToWGS84(ce, cn)
		if !ok {
			continue
		}
		out = append(out, Neighbour{Square: ref.Square, Dir: o.dir, Lat: clat, Lng: clng})
	}
	return out
}
