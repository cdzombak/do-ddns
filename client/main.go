package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/crewjam/errset"
	_ "github.com/joho/godotenv/autoload"
)

type DomainUpdateRequest struct {
	Domain string `json:"domain"`
	Secret string `json:"secret"`
}

const updateInterval = 1 * time.Minute

func mustGetenv(key string) string {
	retv := os.Getenv(key)
	if retv == "" {
		log.Fatalf("environment variable '%s' is missing\n", key)
	}
	return retv
}

func update(endpoint string) error {
	domain := mustGetenv("DDNS_DOMAIN")
	updateBody := DomainUpdateRequest{
		Domain: domain,
		Secret: mustGetenv("DDNS_SECRET"),
	}
	updateJson, err := json.Marshal(updateBody)
	if err != nil {
		log.Fatalf("failed to marshal update request body to JSON: %s", err)
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(updateJson))
	if err != nil {
		log.Fatalf("failed to build update request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")

	const updateTimeout = 10 * time.Second
	client := &http.Client{Timeout: updateTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("update request to '%s' for '%s' failed: %w", endpoint, domain, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("update request to '%s' for '%s' failed with HTTP %s", endpoint, domain, resp.Status)
	}

	return nil
}

func main() {
	ipv4UpdateEndpoint := os.Getenv("DDNS_UPDATE_ENDPOINT_A")
	ipv6UpdateEndpoint := os.Getenv("DDNS_UPDATE_ENDPOINT_AAAA")
	if ipv4UpdateEndpoint == "" && ipv6UpdateEndpoint == "" {
		log.Fatalln("at least one of the environment variables DDNS_UPDATE_ENDPOINT_A and DDNS_UPDATE_ENDPOINT_AAAA must be set")
	}

	t := time.Tick(updateInterval)
	for _ = range t {
		errs := errset.ErrSet{}
		if ipv4UpdateEndpoint != "" {
			if err := update(ipv4UpdateEndpoint); err != nil {
				errs = append(errs, err)
			}
		}
		if ipv6UpdateEndpoint != "" {
			if err := update(ipv6UpdateEndpoint); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) != 0 {
			log.Println(errs.ReturnValue())
		}
	}
}
