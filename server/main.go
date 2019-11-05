package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/crewjam/errset"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	_ "github.com/joho/godotenv/autoload"

	"do-ddns/server/digitalocean"
)

// DomainsConfig is the schema for the configuration file listing domains which may be updated,
// along with their secret keys.
type DomainsConfig struct {
	Domains []DomainConfig `json:"domains"`
}

// DomainConfig represents the configuration for a single domain.
type DomainConfig struct {
	Domain               string `json:"domain"`
	Secret               string `json:"secret"`
	AllowClientIPChoice  bool   `json:"allowClientIPChoice,omitEmpty"` // whether a client-provided IP can be respected, if using an endpoint which allows the client to choose a specific IP
	CreateMissingRecords bool   `json:"createMissingRecords,omitEmpty"` // whether to create missing DNS records, rather than erroring, if no A/AAAA record exists to update
}

func (c *DomainsConfig) findDomain(domain string) (DomainConfig, bool) {
	for _, v := range c.Domains {
		if v.Domain == domain {
			return v, true
		}
	}
	return DomainConfig{}, false
}

// Env describes the application environment (configuration, shared API and cache, etc).
type Env struct {
	DomainsConfig *DomainsConfig
	DOAPI         *digitalocean.APIClient
	UpdateCache   *DNSUpdateCache
	Decoder       *schema.Decoder
}

// DomainUpdateRequest represents a request POSTed by a client to update a domain.
type DomainUpdateRequest DomainConfig

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

type IPVersion int
const (
	IPv4 IPVersion = 4
	IPv6 IPVersion = 6
)

// mustGetenv returns the value of the environment variable with the given name, or exits
// with an error if the variable is empty.
func mustGetenv(key string) string {
	retv := os.Getenv(key)
	if retv == "" {
		log.Fatalf("environment variable '%s' is missing\n", key)
	}
	return retv
}

func main() {
	appEnv := Env{}
	appEnv.UpdateCache = &DNSUpdateCache{}
	appEnv.Decoder = schema.NewDecoder()

	port := os.Getenv("PORT")
	if port == "" {
		log.Println("environment variable 'PORT' is missing; defaulting to 7001")
		port = "7001"
	}

	doAPIKey := mustGetenv("DO_API_KEY")
	appEnv.DOAPI = &digitalocean.APIClient{}
	err := appEnv.DOAPI.SetAPIKey(doAPIKey)
	if err != nil {
		log.Fatalf("failed to initialize DigitalOcean API client: %s\n", err.Error())
	}

	domainsConfigPath := mustGetenv("DOMAINS_CONFIG_PATH")
	configFile, err := ioutil.ReadFile(domainsConfigPath)
	if err != nil {
		log.Fatalf("couldn't read config file '%s': %s", domainsConfigPath, err.Error())
	}
	err = json.Unmarshal(configFile, &appEnv.DomainsConfig)
	if err != nil {
		log.Fatalf("couldn't parse config file '%s' as JSON: %s", domainsConfigPath, err.Error())
	}

	router := mux.NewRouter().StrictSlash(false)
	router.Methods("GET").Path("/ping").Handler(Handler{&appEnv, ping})
	router.Methods("GET").Path("/v3/update").Handler(Handler{&appEnv, dyndnsApiUpdate})
	router.Methods("GET").Path("/nic/update").Handler(Handler{&appEnv, dyndnsApiUpdate})
	router.Methods("POST").Path("/").Handler(Handler{&appEnv, indexPost})
	log.Printf("server is listening on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// ping returns HTTP 204 No Content.
// It is used to check whether the service is running.
func ping(e *Env, w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// indexPost handles POST update requests to the / endpoint, as sent by do-ddns-client.
func indexPost(e *Env, w http.ResponseWriter, r *http.Request) error {
	var updateRequest DomainUpdateRequest
	const oneMB = 1000000
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, oneMB))
	if err != nil {
		return err
	}
	if err := r.Body.Close(); err != nil {
		return err
	}
	if err = json.Unmarshal(body, &updateRequest); err != nil {
		return HandlerError{
			StatusCode: http.StatusUnprocessableEntity,
			Err:        err,
		}
	}

	domainConfig, ok := e.DomainsConfig.findDomain(updateRequest.Domain)
	if !ok {
		return HandlerError{
			StatusCode:  http.StatusNotFound,
			PublicError: fmt.Sprintf("domain '%s' is not configured", updateRequest.Domain),
		}
	}
	if domainConfig.Secret != updateRequest.Secret {
		return HandlerError{
			StatusCode:  http.StatusUnauthorized,
			PublicError: fmt.Sprintf("incorrect secret for domain '%s'", updateRequest.Domain),
		}
	}

	clientIPStr, ipVersion, err := remoteAddr(r)
	if err != nil {
		return err
	}

	recordType := "A"
	if ipVersion == IPv6 {
		recordType = "AAAA"
	}

	if err = performUpdate(e, domainConfig, recordType, clientIPStr); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// dyndnsApiUpdate implements the DynDns update API, allowing do-ddns-server to accept requests from
// routers or other devices with DynDns support built in (such as the Ubiquiti Security Gateway).
// See: https://help.dyn.com/remote-access-api/perform-update/
func dyndnsApiUpdate(e *Env, w http.ResponseWriter, r *http.Request) error {
	var updateRequest DynDnsUpdateRequest
	err := e.Decoder.Decode(&updateRequest, r.URL.Query())
	if err != nil || updateRequest.BackMX != "" || updateRequest.MX != "" || updateRequest.Offline != "" || updateRequest.Wildcard != "" {
		return HandlerError{
			StatusCode:  http.StatusBadRequest,
			Err:         err,
			PublicError: "Invalid query parameters. (Note that backmx, mx, offline, and wildcard are unsupported.)",
		}
	}
	if strings.Contains(updateRequest.Hostnames, ",") {
		return HandlerError{
			StatusCode:  http.StatusBadRequest,
			PublicError: "This server does not support updating multiple hostnames at once.",
		}
	}

	domain := updateRequest.Hostnames
	domainConfig, ok := e.DomainsConfig.findDomain(domain)
	if !ok {
		return HandlerError{
			StatusCode:  http.StatusNotFound,
			PublicError: fmt.Sprintf("domain '%s' is not configured", domain),
		}
	}

	authHdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHdr, "Basic ") {
		return HandlerError{
			StatusCode:  http.StatusUnauthorized,
			Err:         errors.New("authorization header doesn't look like basic auth"),
		}
	}
	auth, err := base64.StdEncoding.DecodeString(authHdr[6:])
	if err != nil {
		return HandlerError{
			StatusCode:  http.StatusUnauthorized,
			Err:         fmt.Errorf("authorization decoding error: %w", err),
		}
	}
	validAuth := fmt.Sprintf("%s:%s", domainConfig.Domain, domainConfig.Secret)
	if string(auth) != validAuth {
		return HandlerError{
			StatusCode:  http.StatusUnauthorized,
			PublicError: fmt.Sprintf("incorrect authorization header for domain '%s' (must be of format 'domain:secret')", domain),
		}
	}

	clientIPStr, clientIPVersion, err := remoteAddr(r)
	if err != nil {
		return err
	}

	updateARecordValue := ""
	updateAAAARecordValue := ""

	if clientIPVersion == IPv4 {
		updateARecordValue = clientIPStr
	} else if clientIPVersion == IPv6 {
		updateAAAARecordValue = clientIPStr
	}

	if (updateRequest.MyIP != clientIPStr) && domainConfig.AllowClientIPChoice {
		// the client IP address and the requested new IP are different, and we're allowed to trust the client's IP choice.
		// see if we can discover both IPv4 and IPv6 addresses from this request; else, just use the client's IP choice.
		myIPVersion, err := ipVersion(updateRequest.MyIP)
		if err != nil {
			return HandlerError{
				StatusCode:  http.StatusBadRequest,
				PublicError: "myip must be a valid ipv4 or ipv6 address",
			}
		}
		if myIPVersion != clientIPVersion {
			// we've discovered IPv4 and IPv6 addresses from this request.
			if myIPVersion == IPv4 {
				updateARecordValue = updateRequest.MyIP
			} else if myIPVersion == IPv6 {
				updateAAAARecordValue = updateRequest.MyIP
			}
			if clientIPVersion == IPv4 {
				updateARecordValue = clientIPStr
			} else if clientIPVersion == IPv6 {
				updateAAAARecordValue = clientIPStr
			}
		} else {
			// the remote address and client's chosen IP are the same IP version, so only use the client's chosen IP.
			if myIPVersion == IPv4 {
				updateARecordValue = updateRequest.MyIP
				updateAAAARecordValue = ""
			} else if myIPVersion == IPv6 {
				updateARecordValue = ""
				updateAAAARecordValue = updateRequest.MyIP
			}
		}
	}

	errs := errset.ErrSet{}

	if updateARecordValue != "" {
		if err = performUpdate(e, domainConfig, "A", updateARecordValue); err != nil {
			errs = append(errs, err)
		}
	}

	if updateAAAARecordValue != "" {
		if err = performUpdate(e, domainConfig, "AAAA", updateAAAARecordValue); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs.ReturnValue()
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func remoteAddr(r *http.Request) (string, IPVersion, error) {
	clientIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid RemoteAddr '%s': %w", r.RemoteAddr, err)
	}
	forwardedFor := r.Header.Get("x-forwarded-for")
	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) != 0 {
			clientIPStr = strings.TrimSpace(parts[0])
		}
	}

	ipVersion, err := ipVersion(clientIPStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid client IP '%s': %w", clientIPStr, err)
	}

	return clientIPStr, ipVersion, nil
}

func ipVersion(ipStr string) (IPVersion, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0, fmt.Errorf("cannot parse IP '%s'", ipStr)
	}

	ipVersion := IPv6
	if p4 := ip.To4(); len(p4) == net.IPv4len {
		ipVersion = IPv4
	}
	return ipVersion, nil
}

func performUpdate(e *Env, c DomainConfig, recordType string, value string) error {
	if e.UpdateCache.Get(c.Domain, recordType) == value {
		log.Printf("cache indicates that %s record for %s is up to date", recordType, c.Domain)
		return nil
	}

	parts := strings.Split(c.Domain, ".")
	if len(parts) < 2 {
		return HandlerError{
			StatusCode:  http.StatusBadRequest,
			Err:         nil,
			PublicError: fmt.Sprintf("'%s' is not a valid domain name", c.Domain),
		}
	}
	rootDomain := strings.Join(parts[len(parts)-2:], ".")
	recordName := "."
	if len(parts) > 2 {
		recordName = strings.Join(parts[:len(parts)-2], ".")
	} else {
		recordName = "@"
	}

	err := e.DOAPI.UpdateRecords(rootDomain, recordName, recordType, value)
	if err == digitalocean.NoMatchingRecordsFoundErr && c.CreateMissingRecords {
		err = e.DOAPI.CreateRecord(rootDomain, recordName, recordType, value)
	}
	if err != nil {
		return HandlerError{
			StatusCode: http.StatusInternalServerError,
			Err:        err,
		}
	}

	e.UpdateCache.Set(c.Domain, recordType, value)
	return nil
}
