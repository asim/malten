package nrail

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// darwin.go is a tiny client for National Rail's OpenLDBWS "Live Departure
// Boards" SOAP service (Darwin). It's dependency-free: we hand-build the SOAP
// envelope and parse the response with encoding/xml, matching on local element
// names so the fiddly RTTI namespaces don't matter.
//
// A free access token is required (register at the National Rail Data Portal /
// Rail Data Marketplace). It's held server-side as NRE_LDBWS_TOKEN, like the
// Anthropic key — never sent to the browser.

const defaultLDBURL = "https://lite.realtime.nationalrail.co.uk/OpenLDBWS/ldb12.asmx"

// Darwin calls the OpenLDBWS departure board.
type Darwin struct {
	Token string
	URL   string
	HTTP  *http.Client
}

// NewDarwin builds a client for the given access token.
func NewDarwin(token string) *Darwin {
	return &Darwin{
		Token: strings.TrimSpace(token),
		URL:   defaultLDBURL,
		HTTP:  &http.Client{Timeout: 12 * time.Second},
	}
}

// Departure is one train leaving a station.
type Departure struct {
	Destination string `json:"destination"`
	Scheduled   string `json:"scheduled"` // std, "HH:MM"
	Expected    string `json:"expected"`  // etd: "On time" | "HH:MM" | "Cancelled" | "Delayed"
	Platform    string `json:"platform,omitempty"`
	Operator    string `json:"operator,omitempty"`
}

// Board is a live departure board for a station.
type Board struct {
	Station    string      `json:"station"`
	CRS        string      `json:"crs"`
	Messages   []string    `json:"messages,omitempty"`
	Departures []Departure `json:"departures"`
}

const envelopeTmpl = `<?xml version="1.0" encoding="utf-8"?>` +
	`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"` +
	` xmlns:typ="http://thalesgroup.com/RTTI/2013-11-28/Token/types"` +
	` xmlns:ldb="http://thalesgroup.com/RTTI/2021-11-01/ldb/">` +
	`<soap:Header><typ:AccessToken><typ:TokenValue>%s</typ:TokenValue></typ:AccessToken></soap:Header>` +
	`<soap:Body><ldb:GetDepartureBoardRequest><ldb:numRows>%d</ldb:numRows><ldb:crs>%s</ldb:crs>` +
	`</ldb:GetDepartureBoardRequest></soap:Body></soap:Envelope>`

// Departures fetches the live departure board for a station CRS code.
func (d *Darwin) Departures(ctx context.Context, crs string, numRows int) (*Board, error) {
	if d.Token == "" {
		return nil, fmt.Errorf("no Darwin token configured")
	}
	crs = strings.ToUpper(strings.TrimSpace(crs))
	if len(crs) != 3 {
		return nil, fmt.Errorf("invalid CRS code")
	}
	if numRows <= 0 || numRows > 20 {
		numRows = 10
	}
	body := fmt.Sprintf(envelopeTmpl, xmlEscape(d.Token), numRows, crs)

	url := d.URL
	if url == "" {
		url = defaultLDBURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "")

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("darwin status %d", resp.StatusCode)
	}
	return parseBoard(data, crs)
}

// --- response parsing (local-name matching ignores the RTTI namespaces) ------

type xmlLocation struct {
	LocationName string `xml:"locationName"`
	CRS          string `xml:"crs"`
}

type xmlService struct {
	STD      string `xml:"std"`
	ETD      string `xml:"etd"`
	Platform string `xml:"platform"`
	Operator string `xml:"operator"`
	Dest     struct {
		Location []xmlLocation `xml:"location"`
	} `xml:"destination"`
}

type xmlBoardResult struct {
	LocationName string `xml:"locationName"`
	CRS          string `xml:"crs"`
	Nrcc         struct {
		Message []string `xml:"message"`
	} `xml:"nrccMessages"`
	TrainServices struct {
		Service []xmlService `xml:"service"`
	} `xml:"trainServices"`
}

type xmlEnvelope struct {
	Body struct {
		Fault *struct {
			FaultString string `xml:"faultstring"`
		} `xml:"Fault"`
		Resp struct {
			Result xmlBoardResult `xml:"GetStationBoardResult"`
		} `xml:"GetDepartureBoardResponse"`
	} `xml:"Body"`
}

var tagRE = regexp.MustCompile(`<[^>]+>`)

func parseBoard(data []byte, crs string) (*Board, error) {
	var env xmlEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Body.Fault != nil && env.Body.Fault.FaultString != "" {
		return nil, fmt.Errorf("darwin: %s", env.Body.Fault.FaultString)
	}
	res := env.Body.Resp.Result
	b := &Board{Station: res.LocationName, CRS: res.CRS}
	if b.CRS == "" {
		b.CRS = crs
	}
	for _, m := range res.Nrcc.Message {
		if t := strings.TrimSpace(tagRE.ReplaceAllString(m, "")); t != "" {
			b.Messages = append(b.Messages, t)
		}
	}
	for _, s := range res.TrainServices.Service {
		dest := ""
		if len(s.Dest.Location) > 0 {
			dest = s.Dest.Location[0].LocationName
		}
		b.Departures = append(b.Departures, Departure{
			Destination: dest,
			Scheduled:   s.STD,
			Expected:    s.ETD,
			Platform:    s.Platform,
			Operator:    s.Operator,
		})
	}
	return b, nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
