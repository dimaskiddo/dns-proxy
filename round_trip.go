package main

import "net/http"

type hybridRoundTripper struct {
	H2Transport http.RoundTripper
	H3Transport http.RoundTripper
}

func (rt *hybridRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Request Using HTTP/3
	reqH3 := req.Clone(req.Context())
	if req.Body != nil && req.GetBody != nil {
		reqH3.Body, _ = req.GetBody()
	}

	resp, err := rt.H3Transport.RoundTrip(reqH3)
	if err != nil {
		// Fallback to HTTP/2
		reqH2 := req.Clone(req.Context())
		if req.Body != nil && req.GetBody != nil {
			reqH2.Body, _ = req.GetBody()
		}

		resp, err = rt.H2Transport.RoundTrip(reqH2)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}
