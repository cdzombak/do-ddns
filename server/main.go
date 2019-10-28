package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
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
	Domain string `json:"domain"`
	Secret string `json:"secret"`
}

func (c *DomainsConfig) findDomain(domain string) *DomainConfig {
	for _, v := range c.Domains {
		if v.Domain == domain {
			return &v
		}
	}
	return nil
}

// Env describes the application environment (configuration, shared API and cache, etc).
type Env struct {
	DomainsConfig *DomainsConfig
	DOAPI         *digitalocean.APIClient
	UpdateCache   *DNSUpdateCache
}

// DomainUpdateRequest represents a request POSTed by a client to update a domain.
type DomainUpdateRequest DomainConfig

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
	router.Methods("POST").Path("/").Handler(Handler{&appEnv, indexPost})
	log.Printf("server is listening on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

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

	domainConfig := e.DomainsConfig.findDomain(updateRequest.Domain)
	if domainConfig == nil {
		return HandlerError{
			StatusCode:  http.StatusNotFound,
			Err:         nil,
			PublicError: fmt.Sprintf("domain '%s' is not configured", updateRequest.Domain),
		}
	}
	if domainConfig.Secret != updateRequest.Secret {
		return HandlerError{
			StatusCode:  http.StatusUnauthorized,
			Err:         nil,
			PublicError: fmt.Sprintf("incorrect secret for domain '%s'", updateRequest.Domain),
		}
	}

	clientIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return fmt.Errorf("invalid RemoteAddr '%s': %w", r.RemoteAddr, err)
	}
	forwardedFor := r.Header.Get("x-forwarded-for")
	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) != 0 {
			clientIPStr = strings.TrimSpace(parts[0])
		}
	}

	clientIP := net.ParseIP(clientIPStr)
	if clientIP == nil {
		return fmt.Errorf("cannot parse IP '%s'", clientIPStr)
	}
	clientIPStr = clientIP.String()
	recordType := "AAAA"
	if p4 := clientIP.To4(); len(p4) == net.IPv4len {
		recordType = "A"
	}

	parts := strings.Split(updateRequest.Domain, ".")
	if len(parts) < 2 {
		return fmt.Errorf("'%s' is not a valid domain name", updateRequest.Domain)
	}
	rootDomain := strings.Join(parts[len(parts)-2:], ".")
	recordName := "."
	if len(parts) > 2 {
		recordName = strings.Join(parts[:len(parts)-2], ".")
	} else {
		recordName = "@"
	}

	if cached := e.UpdateCache.Get(updateRequest.Domain, recordType); cached == clientIPStr {
		w.WriteHeader(204)
		return nil
	}

	err = e.DOAPI.UpdateRecords(rootDomain, recordName, recordType, clientIPStr)
	if err != nil {
		return HandlerError{
			StatusCode: http.StatusInternalServerError,
			Err:        err,
		}
	}

	e.UpdateCache.Set(updateRequest.Domain, recordType, clientIPStr)

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func ping(e *Env, w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}
