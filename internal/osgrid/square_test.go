package osgrid

import "testing"

// OS's own worked example: Caister Water Tower is TG 51409 13177, so its 1 km
// square is TG 51 13.
func TestSquareCaister(t *testing.T) {
	ref, ok := FromWGS84(52.657570, 1.717922)
	if !ok {
		t.Fatal("Caister should be inside the grid")
	}
	if ref.Square != "TG 51 13" {
		t.Errorf("Square = %q, want %q (from %q)", ref.Square, "TG 51 13", ref.GridRef)
	}
}

func TestNeighbours(t *testing.T) {
	// Somewhere inland, so all eight squares exist.
	ns := Neighbours(51.4036, -0.3378) // Hampton Court
	if len(ns) != 8 {
		t.Fatalf("got %d neighbours, want 8: %+v", len(ns), ns)
	}
	here, _ := FromWGS84(51.4036, -0.3378)
	seen := map[string]bool{}
	for _, n := range ns {
		if n.Square == here.Square {
			t.Errorf("neighbours include the square you're in: %q", n.Square)
		}
		if seen[n.Square] {
			t.Errorf("duplicate neighbour %q", n.Square)
		}
		seen[n.Square] = true

		// Each neighbour's centre must land back in the square it claims —
		// this is the round trip through the inverse projection.
		back, ok := FromWGS84(n.Lat, n.Lng)
		if !ok || back.Square != n.Square {
			t.Errorf("%s (%s) centre %.5f,%.5f lands in %q", n.Square, n.Dir, n.Lat, n.Lng, back.Square)
		}
	}

	// North really is north, east really is east.
	byDir := map[string]Neighbour{}
	for _, n := range ns {
		byDir[n.Dir] = n
	}
	if byDir["north"].Lat <= 51.4036 {
		t.Errorf("north neighbour is not north: %+v", byDir["north"])
	}
	if byDir["south"].Lat >= 51.4036 {
		t.Errorf("south neighbour is not south: %+v", byDir["south"])
	}
	if byDir["east"].Lng <= -0.3378 {
		t.Errorf("east neighbour is not east: %+v", byDir["east"])
	}
	if byDir["west"].Lng >= -0.3378 {
		t.Errorf("west neighbour is not west: %+v", byDir["west"])
	}
}

// Outside Great Britain there is no grid, so there are no squares.
func TestNeighboursOutsideGB(t *testing.T) {
	if ns := Neighbours(48.8566, 2.3522); ns != nil { // Paris
		t.Errorf("got %d neighbours outside GB", len(ns))
	}
}
