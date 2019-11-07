package api

// DomainUpdateRequest represents a request POSTed by a client to update a domain.
type DomainUpdateRequest struct {
	Domain               string `json:"domain"`
	Secret               string `json:"secret"`
}

// DynDnsUpdateRequest represents a GET request by the client to the DynDns-style API endpoint.
type DynDnsUpdateRequest struct {
	Hostnames string `schema:"hostname"`
	MyIP      string `schema:"myip"`
	// the following are accepted without error, and ignored:
	System    string `schema:"system"`
	URL       string `schema:"url"`
	// the following are not implemented:
	Wildcard  string `schema:"wildcard"`
	MX        string `schema:"mx"`
	BackMX    string `schema:"backmx"`
	Offline   string `schema:"offline"`
}
