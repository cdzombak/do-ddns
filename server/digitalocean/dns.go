package digitalocean

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
)

// NoRecordsFoundErr indicates that the client failed to find any records for the given domain.
var NoRecordsFoundErr = errors.New("no records found for this domain")

// NoMatchingRecordsFoundErr indicates that the client failed to find any records for the given domain matching the given record name and type.
var NoMatchingRecordsFoundErr = errors.New("no records found for this domain with the given name and type")

// InvalidRecordTypeErr indicates that an invalid record type was specified.
var InvalidRecordTypeErr = errors.New("invalid record type")

// DNSRecord represents a DNS record in the DigitalOcean API.
type DNSRecord struct {
	ID       int64   `json:"id"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Priority *int    `json:"priority"`
	Port     *int    `json:"port"`
	Weight   *int    `json:"weight"`
	TTL      int     `json:"ttl"`
	Flags    *uint8  `json:"flags"`
	Tag      *string `json:"tag"`
	Data     string  `json:"data"`
}

// DNSRecordsResponse represents a DigitalOcean DNS Records response.
type DNSRecordsResponse struct {
	DomainRecords []DNSRecord `json:"domain_records"`
	Meta          struct {
		Total int `json:"total"`
	} `json:"meta"`
	Links struct {
		Pages struct {
			First    string `json:"first"`
			Previous string `json:"prev"`
			Next     string `json:"next"`
			Last     string `json:"last"`
		} `json:"pages"`
	} `json:"links"`
}

// CreateRecordRequest represents a DigitalOcean Create DNS Record request body.
type CreateRecordRequest struct {
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Data     string  `json:"data"`
}

// CreateRecordResponse represents a DigitalOcean Create DNS Record response.
type CreateRecordResponse struct {
	DomainRecord DNSRecord `json:"domain_record"`
}

// GetDomainRecords gets the DNS records of the given domain.
func (c *APIClient) GetDomainRecords(domain string) ([]DNSRecord, error) {
	retv := make([]DNSRecord, 0)
	page := DNSRecordsResponse{}
	uri := APIBase + "/domains/" + url.PathEscape(domain) + "/records"
	for uri != "" {
		if err := c.GetURL(uri, &page); err != nil {
			return nil, err
		}
		retv = append(retv, page.DomainRecords...)
		if uri == page.Links.Pages.Last {
			uri = ""
		} else {
			uri = page.Links.Pages.Last
		}
	}
	return retv, nil
}

// UpdateRecords updates any of the given root domain's records, with the given record name & record type,
// to the given value.
func (c *APIClient) UpdateRecords(rootDomain string, recordName string, recordType string, value string) error {
	log.Printf("updating %s records for '%s.%s' to '%s'\n", recordType, recordName, rootDomain, value)

	doRecords, err := c.GetDomainRecords(rootDomain)
	if err != nil {
		return err
	}
	if len(doRecords) < 1 {
		return NoRecordsFoundErr
	}

	foundRecords := 0
	for _, doRecord := range doRecords {
		if doRecord.Name == recordName && doRecord.Type == recordType {
			foundRecords++
			if doRecord.Data == value {
				continue
			}

			doRecord.Data = value
			update, err := json.Marshal(doRecord)
			if err != nil {
				return fmt.Errorf("failed to marshal record to JSON: %w", err)
			}

			req, err := http.NewRequest("PUT",
				fmt.Sprintf("%s/domains/%s/records/%d", APIBase, url.PathEscape(rootDomain), doRecord.ID),
				bytes.NewBuffer(update))
			if err != nil {
				return fmt.Errorf("failed to build update request: %w", err)
			}

			_, err = c.Do(req)
			if err != nil {
				return fmt.Errorf("update failed: %w", err)
			}
		}
	}

	if foundRecords == 0 {
		return NoMatchingRecordsFoundErr
	}

	return nil
}

// CreateRecord creates a DNS record according to the given values. Currently only A and AAAA records are supported.
func (c *APIClient) CreateRecord(rootDomain string, recordName string, recordType string, value string) error {
	if recordType != "A" && recordType != "AAAA" {
		return InvalidRecordTypeErr
	}

	reqJSON, err := json.Marshal(CreateRecordRequest{
		Type: recordType,
		Name: recordName,
		Data: value,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request to JSON: %w", err)
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/domains/%s/records", APIBase, url.PathEscape(rootDomain)),
		bytes.NewBuffer(reqJSON))
	if err != nil {
		return fmt.Errorf("failed to build create request: %w", err)
	}

	_, err = c.Do(req)
	if err != nil {
		return fmt.Errorf("create failed: %w", err)
	}

	return nil
}
