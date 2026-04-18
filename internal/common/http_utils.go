package common

import "net/http"

// EndpointReturnsExpected wraps a GET request in a caller-provided readiness predicate.
func EndpointReturnsExpected(client *http.Client, endpoint string, expected func(resp *http.Response, err error) bool) bool {
	resp, err := client.Get(endpoint)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	return expected(resp, err)
}

// IsEndpointReachable reports whether an endpoint responds without a server-side failure.
func IsEndpointReachable(client *http.Client, endpoint string) bool {
	return EndpointReturnsExpected(client, endpoint, func(resp *http.Response, err error) bool {
		return err == nil && resp != nil && resp.StatusCode < http.StatusInternalServerError
	})
}

// IsJSONEndpointReady reports whether an endpoint is ready to serve successful JSON responses.
func IsJSONEndpointReady(client *http.Client, endpoint string) bool {
	return EndpointReturnsExpected(client, endpoint, func(resp *http.Response, err error) bool {
		return err == nil && resp != nil && resp.StatusCode == http.StatusOK
	})
}
