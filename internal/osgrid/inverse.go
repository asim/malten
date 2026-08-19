package osgrid

import "math"

// inverse.go adds the reverse of FromWGS84: National Grid easting/northing back
// to WGS84 latitude/longitude. It's the mirror pipeline — inverse Transverse
// Mercator to Airy 1830 lat/lon, then an inverse Helmert transform (OSGB36 ->
// WGS84) — and is what lets us place OS-sourced results (OS Names, which returns
// British National Grid only) onto the WGS84 map.

// ToWGS84 converts a National Grid easting/northing (metres) to WGS84 lat/lon in
// degrees. ok is false if the coordinate is outside the National Grid extent.
func ToWGS84(easting, northing float64) (lat, lon float64, ok bool) {
	if easting < 0 || easting >= 700000 || northing < 0 || northing >= 1300000 {
		return 0, 0, false
	}
	aLat, aLon := enToAiry(easting, northing)
	wLat, wLon := airyToWGS84(aLat, aLon)
	return wLat * 180 / math.Pi, wLon * 180 / math.Pi, true
}

// enToAiry inverts the Transverse Mercator projection: National Grid
// easting/northing -> Airy 1830 lat/lon (radians). Uses the OS inverse series.
func enToAiry(easting, northing float64) (lat, lon float64) {
	a, b := airy.a, airy.b
	e2 := (a*a - b*b) / (a * a)
	n := (a - b) / (a + b)
	n2, n3 := n*n, n*n*n

	// Iterate latitude until the meridional arc matches the northing.
	lat = natGridLat0
	m := 0.0
	for i := 0; i < 100; i++ {
		lat = (northing-natGridN0-m)/(a*natGridF0) + lat
		dLat := lat - natGridLat0
		sLat := lat + natGridLat0
		m = b * natGridF0 * ((1+n+1.25*n2+1.25*n3)*dLat -
			(3*n+3*n2+2.625*n3)*math.Sin(dLat)*math.Cos(sLat) +
			(1.875*n2+1.875*n3)*math.Sin(2*dLat)*math.Cos(2*sLat) -
			(35.0/24*n3)*math.Sin(3*dLat)*math.Cos(3*sLat))
		if math.Abs(northing-natGridN0-m) < 1e-5 {
			break
		}
	}

	sinLat := math.Sin(lat)
	nu := a * natGridF0 / math.Sqrt(1-e2*sinLat*sinLat)
	rho := a * natGridF0 * (1 - e2) / math.Pow(1-e2*sinLat*sinLat, 1.5)
	eta2 := nu/rho - 1

	tanLat := math.Tan(lat)
	tan2 := tanLat * tanLat
	tan4 := tan2 * tan2
	tan6 := tan4 * tan2
	secLat := 1 / math.Cos(lat)
	nu3 := nu * nu * nu
	nu5 := nu3 * nu * nu
	nu7 := nu5 * nu * nu

	vii := tanLat / (2 * rho * nu)
	viii := tanLat / (24 * rho * nu3) * (5 + 3*tan2 + eta2 - 9*tan2*eta2)
	ix := tanLat / (720 * rho * nu5) * (61 + 90*tan2 + 45*tan4)
	x := secLat / nu
	xi := secLat / (6 * nu3) * (nu/rho + 2*tan2)
	xii := secLat / (120 * nu5) * (5 + 28*tan2 + 24*tan4)
	xiia := secLat / (5040 * nu7) * (61 + 662*tan2 + 1320*tan4 + 720*tan6)

	dE := easting - natGridE0
	dE2 := dE * dE
	lat = lat - vii*dE2 + viii*dE2*dE2 - ix*dE2*dE2*dE2
	lon = natGridLon0 + x*dE - xi*dE*dE2 + xii*dE*dE2*dE2 - xiia*dE*dE2*dE2*dE2
	return lat, lon
}

// airyToWGS84 is the inverse Helmert transform (OSGB36/Airy -> WGS84). The
// parameters are the negation of the WGS84 -> OSGB36 set used in FromWGS84,
// which inverts the small transform to well within its own few-metre accuracy.
func airyToWGS84(lat, lon float64) (float64, float64) {
	x, y, z := toCartesian(airy, lat, lon)

	const (
		tx = 446.448     // metres
		ty = -125.157    // metres
		tz = 542.060     // metres
		s  = -20.4894e-6 // scale (ppm)
		rx = 0.1502 / 3600 * math.Pi / 180
		ry = 0.2470 / 3600 * math.Pi / 180
		rz = 0.8421 / 3600 * math.Pi / 180
	)
	x2 := tx + x*(1+s) - y*rz + z*ry
	y2 := ty + x*rz + y*(1+s) - z*rx
	z2 := tz - x*ry + y*rx + z*(1+s)

	return fromCartesian(wgs84, x2, y2, z2)
}
