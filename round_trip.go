package main

import "net/http"

type hybridRoundTripper struct {
	H2Transport http.RoundTripper
	H3Transport http.RoundTripper
}

func (rt *hybridRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Prepare the request for HTTP/3
	reqH3 := req.Clone(req.Context())
	if req.Body != nil && req.GetBody != nil {
		reqH3.Body, _ = req.GetBody()
	}

	// 1. Attempt HTTP/3 (QUIC) First
	resp, err := rt.H3Transport.RoundTrip(reqH3)
	if err == nil {
		return resp, nil
	}

	// 2. Fallback to HTTP/2 (TCP) if HTTP/3 fails
	reqH2 := req.Clone(req.Context())
	if req.Body != nil && req.GetBody != nil {
		reqH2.Body, _ = req.GetBody()
	}

	return rt.H2Transport.RoundTrip(reqH2)
}
