// Package osgrid converts WGS84 latitude/longitude (what a phone's GPS gives)
// into an Ordnance Survey National Grid reference — the coordinate system
// printed on every OS map (e.g. "TG 51409 13177").
//
// The pipeline is the standard one: WGS84 lat/lon -> geocentric cartesian ->
// Helmert 7-parameter transform to OSGB36 -> Airy 1830 lat/lon -> Transverse
// Mercator projection to easting/northing -> grid letters + digits. It uses a
// Helmert transform (not the OSTN15 grid shift), so it is accurate to a few
// metres — ample for a human-readable grid reference.
package osgrid

import (
	"fmt"
	"math"
)

// Ref is a National Grid reference for a point.
type Ref struct {
	GridRef  string `json:"grid_ref"` // e.g. "TG 51409 13177"
	Square   string `json:"square"`   // the 1 km square it falls in, e.g. "TG 51 13"
	Easting  int    `json:"easting"`  // metres from the grid origin
	Northing int    `json:"northing"`
}

type ellipsoid struct{ a, b float64 }

var (
	wgs84 = ellipsoid{a: 6378137.0, b: 6356752.314245}
	airy  = ellipsoid{a: 6377563.396, b: 6356256.909}
)

// National Grid Transverse Mercator parameters (on the Airy 1830 ellipsoid).
const (
	natGridF0   = 0.9996012717         // central meridian scale factor
	natGridLat0 = 49.0 * math.Pi / 180 // true origin latitude (49°N)
	natGridLon0 = -2.0 * math.Pi / 180 // true origin longitude (2°W)
	natGridE0   = 400000.0             // true origin easting
	natGridN0   = -100000.0            // true origin northing
)

// FromWGS84 returns the National Grid reference for a WGS84 lat/lon. ok is false
// if the point falls outside the National Grid (i.e. outside Great Britain).
func FromWGS84(lat, lon float64) (Ref, bool) {
	latR, lonR := wgs84ToAiry(lat*math.Pi/180, lon*math.Pi/180)
	e, n := airyToEN(latR, lonR)
	return enToRef(e, n)
}

// wgs84ToAiry converts WGS84 lat/lon (radians) to OSGB36/Airy lat/lon (radians)
// via a geocentric Helmert transform.
func wgs84ToAiry(lat, lon float64) (float64, float64) {
	x, y, z := toCartesian(wgs84, lat, lon)

	// Helmert transform, WGS84 -> OSGB36.
	const (
		tx = -446.448   // metres
		ty = 125.157    // metres
		tz = -542.060   // metres
		s  = 20.4894e-6 // scale (ppm)
		// rotations, arc-seconds -> radians
		rx = -0.1502 / 3600 * math.Pi / 180
		ry = -0.2470 / 3600 * math.Pi / 180
		rz = -0.8421 / 3600 * math.Pi / 180
	)
	x2 := tx + x*(1+s) - y*rz + z*ry
	y2 := ty + x*rz + y*(1+s) - z*rx
	z2 := tz - x*ry + y*rx + z*(1+s)

	return fromCartesian(airy, x2, y2, z2)
}

func toCartesian(el ellipsoid, lat, lon float64) (x, y, z float64) {
	a, b := el.a, el.b
	e2 := (a*a - b*b) / (a * a)
	sinLat, cosLat := math.Sin(lat), math.Cos(lat)
	nu := a / math.Sqrt(1-e2*sinLat*sinLat)
	x = nu * cosLat * math.Cos(lon)
	y = nu * cosLat * math.Sin(lon)
	z = (1 - e2) * nu * sinLat
	return
}

func fromCartesian(el ellipsoid, x, y, z float64) (lat, lon float64) {
	a, b := el.a, el.b
	e2 := (a*a - b*b) / (a * a)
	p := math.Hypot(x, y)
	lat = math.Atan2(z, p*(1-e2))
	for i := 0; i < 10; i++ {
		sinLat := math.Sin(lat)
		nu := a / math.Sqrt(1-e2*sinLat*sinLat)
		lat = math.Atan2(z+e2*nu*sinLat, p)
	}
	lon = math.Atan2(y, x)
	return
}

// airyToEN projects Airy 1830 lat/lon (radians) to National Grid easting and
// northing using the OS Transverse Mercator series.
func airyToEN(lat, lon float64) (float64, float64) {
	a, b := airy.a, airy.b
	e2 := (a*a - b*b) / (a * a)
	n := (a - b) / (a + b)
	n2, n3 := n*n, n*n*n

	sinLat, cosLat, tanLat := math.Sin(lat), math.Cos(lat), math.Tan(lat)
	nu := a * natGridF0 / math.Sqrt(1-e2*sinLat*sinLat)
	rho := a * natGridF0 * (1 - e2) / math.Pow(1-e2*sinLat*sinLat, 1.5)
	eta2 := nu/rho - 1

	dLat := lat - natGridLat0
	sLat := lat + natGridLat0
	m := b * natGridF0 * ((1+n+1.25*n2+1.25*n3)*dLat -
		(3*n+3*n2+2.625*n3)*math.Sin(dLat)*math.Cos(sLat) +
		(1.875*n2+1.875*n3)*math.Sin(2*dLat)*math.Cos(2*sLat) -
		(35.0/24*n3)*math.Sin(3*dLat)*math.Cos(3*sLat))

	cos3 := cosLat * cosLat * cosLat
	cos5 := cos3 * cosLat * cosLat
	tan2 := tanLat * tanLat
	tan4 := tan2 * tan2

	one := m + natGridN0
	two := nu / 2 * sinLat * cosLat
	three := nu / 24 * sinLat * cos3 * (5 - tan2 + 9*eta2)
	threeA := nu / 720 * sinLat * cos5 * (61 - 58*tan2 + tan4)
	four := nu * cosLat
	five := nu / 6 * cos3 * (nu/rho - tan2)
	six := nu / 120 * cos5 * (5 - 18*tan2 + tan4 + 14*eta2 - 58*tan2*eta2)

	dLon := lon - natGridLon0
	d2 := dLon * dLon
	northing := one + two*d2 + three*d2*d2 + threeA*d2*d2*d2
	easting := natGridE0 + four*dLon + five*dLon*d2 + six*dLon*d2*d2
	return easting, northing
}

// enToRef converts easting/northing to a 10-figure (1 m) grid reference.
func enToRef(easting, northing float64) (Ref, bool) {
	e100k := int(math.Floor(easting / 100000))
	n100k := int(math.Floor(northing / 100000))
	if e100k < 0 || e100k > 6 || n100k < 0 || n100k > 12 {
		return Ref{}, false // outside the National Grid (outside GB)
	}

	// First letter: 500 km grid square; second: 100 km square. 'I' is skipped.
	l1 := (19 - n100k) - (19-n100k)%5 + (e100k+10)/5
	l2 := (19-n100k)*5%25 + e100k%5
	letter := func(n int) byte {
		if n > 7 { // skip 'I'
			n++
		}
		return byte('A' + n)
	}
	letters := string([]byte{letter(l1), letter(l2)})

	e := int(math.Floor(easting)) % 100000
	n := int(math.Floor(northing)) % 100000
	return Ref{
		GridRef:  fmt.Sprintf("%s %05d %05d", letters, e, n),
		Square:   fmt.Sprintf("%s %02d %02d", letters, e/1000, n/1000),
		Easting:  int(math.Round(easting)),
		Northing: int(math.Round(northing)),
	}, true
}
