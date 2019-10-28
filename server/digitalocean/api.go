package digitalocean

// DigitalOcean API code is adapted from https://github.com/anaganisk/digitalocean-dynamic-dns-ip

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const APIBase = "https://api.digitalocean.com/v2"

// APIClient is a client for the DigitalOcean API.
type APIClient struct {
	httpClient *http.Client
}

// APIError represents an error from the DigitalOcean API.
// It confirms to the Golang error interface.
type APIError struct {
	Code    int    `json:"-"`
	ID      string `json:"id"`
	Message string `json:"message"`
}

// Error conforms APIError to the Golang error interface.
func (e APIError) Error() string {
	return fmt.Sprintf("HTTP %d %s: (%s) %s",
		e.Code,
		http.StatusText(e.Code),
		e.ID,
		e.Message)
}

// SetAPIKey authenticates this API client with the given API key.
// It performs a simple check that the API key works, and returns an error if it doesn't.
func (c *APIClient) SetAPIKey(apiKey string) error {
	const apiTimeout = 5 * time.Second
	httpClient := &http.Client{Timeout: apiTimeout}
	rt := withHeader(httpClient.Transport)
	rt.Set("Authorization", "Bearer "+apiKey)
	rt.Set("Accept", "application/json")
	rt.Set("Content-Type", "application/json")
	httpClient.Transport = rt
	c.httpClient = httpClient

	err := c.GetURL(APIBase+"/account", nil)
	if err != nil {
		return err
	}

	return nil
}

// headerSettingRoundTripper is a RoundTripper transport which automatically sets headers on every request.
type headerSettingRoundTripper struct {
	http.Header
	rt http.RoundTripper
}

func withHeader(rt http.RoundTripper) headerSettingRoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return headerSettingRoundTripper{
		Header: make(http.Header),
		rt:     rt,
	}
}

// RoundTrip conforms headerSettingRoundTripper to http.RoundTripper.
func (h headerSettingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.Header {
		req.Header[k] = v
	}
	return h.rt.RoundTrip(req)
}

// Do performs the given request against the DigitalOcean API.
func (c *APIClient) Do(r *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(r)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode >= 400 {
		var doErr APIError
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return resp, fmt.Errorf("failed to read response: %w", err)
		}
		err = json.Unmarshal(body, &doErr)
		if err != nil {
			return resp, fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		doErr.Code = resp.StatusCode
		return resp, doErr
	}

	return resp, nil
}

// GetURL gets the content of the given URL from the DigitalOcean API, unmarshaling the JSON
// response into the given respBody (if it's not nil).
func (c *APIClient) GetURL(url string, respBody interface{}) error {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("can't build request for '%s': %w", url, err)
	}

	response, err := c.Do(request)
	if err != nil {
		return fmt.Errorf("failed to GET '%s': %w", url, err)
	}

	if respBody != nil {
		defer response.Body.Close()
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("failed to read response from '%s': %w", url, err)
		}
		err = json.Unmarshal(body, &respBody)
		if err != nil {
			return fmt.Errorf("failed to unmarshal JSON response from '%s': %w", url, err)
		}
	}

	return nil
}

