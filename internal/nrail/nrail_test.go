package nrail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStationsLoaded(t *testing.T) {
	if Count() < 2000 {
		t.Fatalf("expected the full GB station set, got %d", Count())
	}
	if _, ok := Lookup("KGX"); !ok {
		t.Errorf("King's Cross (KGX) should be in the dataset")
	}
}

// TestNearestIsSelf queries a station's own coordinates; it should come back
// first with a near-zero distance.
func TestNearestIsSelf(t *testing.T) {
	kgx, ok := Lookup("KGX")
	if !ok {
		t.Skip("KGX not present")
	}
	got := Nearest(kgx.Lat, kgx.Lng, 3)
	if len(got) == 0 {
		t.Fatal("no nearest stations returned")
	}
	if got[0].CRS != "KGX" {
		t.Errorf("nearest to KGX = %s, want KGX", got[0].CRS)
	}
	if got[0].Miles > 0.1 {
		t.Errorf("distance to self = %.3f mi, want ~0", got[0].Miles)
	}
	// St Pancras (STP) is next door — it should be within a mile.
	var stpMiles = -1.0
	for _, s := range Nearest(kgx.Lat, kgx.Lng, 8) {
		if s.CRS == "STP" {
			stpMiles = s.Miles
		}
	}
	if stpMiles < 0 || stpMiles > 1.0 {
		t.Errorf("St Pancras should be within a mile of King's Cross, got %.3f", stpMiles)
	}
}

// A realistic-shaped OpenLDBWS response, with the RTTI-style namespace prefixes
// declared, to prove local-name matching survives them.
const sampleResponse = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
 <soap:Body>
  <GetDepartureBoardResponse xmlns="http://thalesgroup.com/RTTI/2021-11-01/ldb/">
   <GetStationBoardResult xmlns:lt4="http://thalesgroup.com/RTTI/2021-11-01/ldb/types" xmlns:lt5="http://thalesgroup.com/RTTI/2021-11-01/ldb/types">
    <lt4:locationName>London Kings Cross</lt4:locationName>
    <lt4:crs>KGX</lt4:crs>
    <lt4:nrccMessages><lt4:message>Engineering work &lt;b&gt;this weekend&lt;/b&gt;.</lt4:message></lt4:nrccMessages>
    <lt5:trainServices>
     <lt5:service>
      <lt4:std>12:00</lt4:std>
      <lt4:etd>On time</lt4:etd>
      <lt4:platform>1</lt4:platform>
      <lt4:operator>LNER</lt4:operator>
      <lt5:destination><lt4:location><lt4:locationName>Edinburgh</lt4:locationName><lt4:crs>EDB</lt4:crs></lt4:location></lt5:destination>
     </lt5:service>
     <lt5:service>
      <lt4:std>12:03</lt4:std>
      <lt4:etd>12:09</lt4:etd>
      <lt4:platform>8</lt4:platform>
      <lt4:operator>Thameslink</lt4:operator>
      <lt5:destination><lt4:location><lt4:locationName>Cambridge</lt4:locationName><lt4:crs>CBG</lt4:crs></lt4:location></lt5:destination>
     </lt5:service>
    </lt5:trainServices>
   </GetStationBoardResult>
  </GetDepartureBoardResponse>
 </soap:Body>
</soap:Envelope>`

func TestParseBoard(t *testing.T) {
	b, err := parseBoard([]byte(sampleResponse), "KGX")
	if err != nil {
		t.Fatalf("parseBoard: %v", err)
	}
	if b.Station != "London Kings Cross" || b.CRS != "KGX" {
		t.Errorf("station = %q/%q", b.Station, b.CRS)
	}
	if len(b.Messages) != 1 || b.Messages[0] != "Engineering work this weekend." {
		t.Errorf("messages not stripped/parsed: %#v", b.Messages)
	}
	if len(b.Departures) != 2 {
		t.Fatalf("departures = %d, want 2", len(b.Departures))
	}
	d0 := b.Departures[0]
	if d0.Destination != "Edinburgh" || d0.Scheduled != "12:00" || d0.Expected != "On time" || d0.Platform != "1" || d0.Operator != "LNER" {
		t.Errorf("first departure wrong: %#v", d0)
	}
	if b.Departures[1].Expected != "12:09" {
		t.Errorf("second departure etd = %q", b.Departures[1].Expected)
	}
}

func TestDeparturesHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") == "" {
			t.Errorf("missing content-type")
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(sampleResponse))
	}))
	defer srv.Close()

	d := NewDarwin("test-token")
	d.URL = srv.URL
	b, err := d.Departures(context.Background(), "kgx", 10)
	if err != nil {
		t.Fatalf("Departures: %v", err)
	}
	if len(b.Departures) != 2 {
		t.Fatalf("got %d departures", len(b.Departures))
	}
}

func TestDeparturesNoToken(t *testing.T) {
	d := NewDarwin("")
	if _, err := d.Departures(context.Background(), "KGX", 10); err == nil {
		t.Error("expected error with no token")
	}
}
