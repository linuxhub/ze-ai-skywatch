package network

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestLookupPrivacyValidationAndRateLimit(t *testing.T) {
	calls := 0
	status := 200
	body := `{"ip":"8.8.8.8","success":true,"country":"United States","city":"Example","latitude":37.4,"longitude":-122.1,"connection":{"asn":15169,"org":"Google LLC"}}`
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" || req.URL.String() != "https://ipwho.is/8.8.8.8" || req.Body != nil || len(req.Header) != 0 {
			t.Fatalf("unexpected lookup disclosure: %v", req)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	if got := Lookup(context.Background(), client, "127.0.0.1"); got.Known || calls != 0 {
		t.Fatal("private IP must not leave the app")
	}
	if got := Lookup(context.Background(), client, "8.8.8.8"); !got.Known || got.ASN != 15169 || calls != 1 {
		t.Fatal(got)
	}
	body = `{"ip":"1.1.1.1","success":true,"country":"Wrong address"}`
	if got := Lookup(context.Background(), client, "8.8.8.8"); got.Known {
		t.Fatal("accepted a mismatched response IP")
	}
	status = 429
	if got := Lookup(context.Background(), client, "8.8.8.8"); !got.Limited || got.Known {
		t.Fatal(got)
	}
}
