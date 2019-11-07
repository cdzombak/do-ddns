package app

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"

	"do-ddns/server/cache"
	"do-ddns/server/digitalocean"

	"github.com/gorilla/schema"
)

// Env describes the application environment (configuration, shared API and cache, etc).
type Env struct {
	domainsConfig     *DomainsConfig
	domainsConfigLock sync.RWMutex
	DOAPI             *digitalocean.APIClient
	UpdateCache       *cache.DNSUpdateCache
	Decoder           *schema.Decoder
}

// DomainsConfig is the schema for the configuration file listing domains that may be updated,
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

// DomainConfig looks up the configuration for the given domain name.
func (e *Env) DomainConfig(domain string) (DomainConfig, bool) {
	e.domainsConfigLock.RLock()
	defer e.domainsConfigLock.RUnlock()
	for _, v := range e.domainsConfig.Domains {
		if v.Domain == domain {
			return v, true
		}
	}
	return DomainConfig{}, false
}

// ReadDomainsConfig updates the environment's domain configuration, reading it from the given path.
func (e *Env) ReadDomainsConfig(configPath string) error {
	e.domainsConfigLock.Lock()
	defer e.domainsConfigLock.Unlock()

	configFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("couldn't read config file '%s': %w", configPath, err)
	}
	err = json.Unmarshal(configFile, &e.domainsConfig)
	if err != nil {
		return fmt.Errorf("couldn't parse config file '%s' as JSON: %w", configPath, err)
	}
	return nil
}
