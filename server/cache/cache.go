package cache

import (
	"fmt"
	"sync"
	"time"
)

const cacheLifetime = 10 * time.Minute

type cacheEntry struct {
	value   string
	expires time.Time
}

// DNSUpdateCache caches the client IP for a given domain/record type pair for 10 minutes.
// This allows us to avoid re-checking the DigitalOcean API every minute, even when clients
// call the update API that frequently.
//
// The TTL for these records can therefore be set to 5 minutes, or 300 seconds.
type DNSUpdateCache struct {
	dnsCacheMutex sync.Mutex
	dnsCache      map[string]cacheEntry
}

// Get retrieves the non-expired value stored for the given domain/record type pair,
// or the empty string if the entry is missing or expired.
func (c *DNSUpdateCache) Get(domain string, recordType string) string {
	c.dnsCacheMutex.Lock()
	defer c.dnsCacheMutex.Unlock()

	key := cacheKey(domain, recordType)
	entry := c.dnsCache[key]
	if time.Now().After(entry.expires) {
		delete(c.dnsCache, key)
		return ""
	}

	return entry.value
}

// Set caches the given value for the given domain/record type pair, with a 10-minute TTL.
func (c *DNSUpdateCache) Set(domain string, recordType string, value string) {
	c.dnsCacheMutex.Lock()
	defer c.dnsCacheMutex.Unlock()

	if c.dnsCache == nil {
		c.dnsCache = make(map[string]cacheEntry)
	}

	key := cacheKey(domain, recordType)
	c.dnsCache[key] = cacheEntry{
		value:   value,
		expires: time.Now().Add(cacheLifetime),
	}
}

func cacheKey(domain string, recordType string) string {
	return fmt.Sprintf("%s:%s", domain, recordType)
}