package spatial

// Geohash encodes lat/lon into a string
// Precision 6 = ~1.2km x 0.6km cells
// Precision 7 = ~150m x 150m cells
func Geohash(lat, lon float64, precision int) string {
	const base32 = "0123456789bcdefghjkmnpqrstuvwxyz"

	minLat, maxLat := -90.0, 90.0
	minLon, maxLon := -180.0, 180.0

	var hash []byte
	var bit int
	var ch byte
	even := true

	for len(hash) < precision {
		if even {
			mid := (minLon + maxLon) / 2
			if lon >= mid {
				ch |= 1 << (4 - bit)
				minLon = mid
			} else {
				maxLon = mid
			}
		} else {
			mid := (minLat + maxLat) / 2
			if lat >= mid {
				ch |= 1 << (4 - bit)
				minLat = mid
			} else {
				maxLat = mid
			}
		}
		even = !even

		bit++
		if bit == 5 {
			hash = append(hash, base32[ch])
			bit = 0
			ch = 0
		}
	}

	return string(hash)
}

// StreamFromLocation returns the stream ID for a location
// Uses geohash precision 6 (~1km cells)
func StreamFromLocation(lat, lon float64) string {
	return Geohash(lat, lon, 6)
}
