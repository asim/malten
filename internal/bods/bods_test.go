package bods

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleSiri = `<?xml version="1.0" encoding="UTF-8"?>
<Siri xmlns="http://www.siri.org.uk/siri" version="2.0">
 <ServiceDelivery>
  <ResponseTimestamp>2026-08-19T12:00:00Z</ResponseTimestamp>
  <VehicleMonitoringDelivery>
   <VehicleActivity>
    <RecordedAtTime>2026-08-19T11:59:50Z</RecordedAtTime>
    <MonitoredVehicleJourney>
     <LineRef>1</LineRef>
     <PublishedLineName>1</PublishedLineName>
     <OperatorRef>FBRI</OperatorRef>
     <DestinationName>Cribbs Causeway</DestinationName>
     <VehicleLocation><Longitude>-2.59100</Longitude><Latitude>51.45400</Latitude></VehicleLocation>
     <Bearing>270</Bearing>
     <VehicleRef>FBRI-33001</VehicleRef>
    </MonitoredVehicleJourney>
   </VehicleActivity>
   <VehicleActivity>
    <MonitoredVehicleJourney>
     <LineRef>75</LineRef>
     <OperatorRef>FBRI</OperatorRef>
     <DestinationName>Hengrove</DestinationName>
     <VehicleLocation><Longitude>-2.58000</Longitude><Latitude>51.44000</Latitude></VehicleLocation>
    </MonitoredVehicleJourney>
   </VehicleActivity>
   <VehicleActivity>
    <MonitoredVehicleJourney>
     <LineRef>bad</LineRef>
     <VehicleLocation><Longitude></Longitude><Latitude></Latitude></VehicleLocation>
    </MonitoredVehicleJourney>
   </VehicleActivity>
  </VehicleMonitoringDelivery>
 </ServiceDelivery>
</Siri>`

func TestParseVehicles(t *testing.T) {
	buses, err := parseVehicles([]byte(sampleSiri), 100)
	if err != nil {
		t.Fatalf("parseVehicles: %v", err)
	}
	// The third activity has no coordinates and must be skipped.
	if len(buses) != 2 {
		t.Fatalf("got %d buses, want 2", len(buses))
	}
	b := buses[0]
	if b.Line != "1" || b.Destination != "Cribbs Causeway" || b.Operator != "FBRI" {
		t.Errorf("first bus wrong: %#v", b)
	}
	if b.Lat != 51.454 || b.Lng != -2.591 || b.Bearing != 270 {
		t.Errorf("first bus position wrong: %#v", b)
	}
	// Second uses LineRef as fallback for the line name.
	if buses[1].Line != "75" {
		t.Errorf("second bus line = %q, want 75", buses[1].Line)
	}
}

func TestVehiclesCap(t *testing.T) {
	buses, err := parseVehicles([]byte(sampleSiri), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(buses) != 1 {
		t.Errorf("cap not applied: got %d", len(buses))
	}
}

func TestVehiclesHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "test-key" {
			t.Errorf("missing api_key")
		}
		if r.URL.Query().Get("boundingBox") == "" {
			t.Errorf("missing boundingBox")
		}
		_, _ = w.Write([]byte(sampleSiri))
	}))
	defer srv.Close()

	c := New("test-key")
	c.URL = srv.URL
	buses, err := c.Vehicles(context.Background(), -2.6, 51.4, -2.5, 51.5, 0)
	if err != nil {
		t.Fatalf("Vehicles: %v", err)
	}
	if len(buses) != 2 {
		t.Fatalf("got %d buses", len(buses))
	}
}

func TestVehiclesNoKey(t *testing.T) {
	c := New("")
	if _, err := c.Vehicles(context.Background(), 0, 0, 1, 1, 0); err == nil {
		t.Error("expected error with no key")
	}
}
