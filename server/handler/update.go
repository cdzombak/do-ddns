package handler

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
	"strings"

	"do-ddns/server/api"
	"do-ddns/server/app"
	"do-ddns/server/digitalocean"

	"github.com/crewjam/errset"
)

type IPVersion int
const (
	IPv4 IPVersion = 4
	IPv6 IPVersion = 6
)

// PostUpdate handles POST update requests to the / endpoint, as sent by do-ddns-client.
func PostUpdate(e *app.Env, w http.ResponseWriter, r *http.Request) error {
	var updateRequest api.DomainUpdateRequest
	const oneMB = 1000000
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, oneMB))
	if err != nil {
		return err
	}
	if err := r.Body.Close(); err != nil {
		return err
	}
	if err = json.Unmarshal(body, &updateRequest); err != nil {
		return app.HandlerError{
			StatusCode: http.StatusUnprocessableEntity,
			Err:        err,
		}
	}

	domainConfig, ok := e.DomainConfig(updateRequest.Domain)
	if !ok {
		return app.HandlerError{
			StatusCode:  http.StatusNotFound,
			PublicError: fmt.Sprintf("domain '%s' is not configured", updateRequest.Domain),
		}
	}
	if domainConfig.Secret != updateRequest.Secret {
		return app.HandlerError{
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

// DynDnsApiUpdate implements the DynDns update API, allowing do-ddns-server to accept requests from
// routers or other devices with DynDns support built in (such as the Ubiquiti Security Gateway).
// See: https://help.dyn.com/remote-access-api/perform-update/
func DynDnsApiUpdate(e *app.Env, w http.ResponseWriter, r *http.Request) error {
	var updateRequest api.DynDnsUpdateRequest
	err := e.Decoder.Decode(&updateRequest, r.URL.Query())
	if err != nil || updateRequest.BackMX != "" || updateRequest.MX != "" || updateRequest.Offline != "" || updateRequest.Wildcard != "" {
		return app.HandlerError{
			StatusCode:  http.StatusBadRequest,
			Err:         err,
			PublicError: "Invalid query parameters. (Note that backmx, mx, offline, and wildcard are unsupported.)",
		}
	}
	if strings.Contains(updateRequest.Hostnames, ",") {
		return app.HandlerError{
			StatusCode:  http.StatusBadRequest,
			PublicError: "This server does not support updating multiple hostnames at once.",
		}
	}

	domain := updateRequest.Hostnames
	domainConfig, ok := e.DomainConfig(domain)
	if !ok {
		return app.HandlerError{
			StatusCode:  http.StatusNotFound,
			PublicError: fmt.Sprintf("domain '%s' is not configured", domain),
		}
	}

	authHdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHdr, "Basic ") {
		return app.HandlerError{
			StatusCode:  http.StatusUnauthorized,
			Err:         errors.New("authorization header doesn't look like basic auth"),
		}
	}
	auth, err := base64.StdEncoding.DecodeString(authHdr[6:])
	if err != nil {
		return app.HandlerError{
			StatusCode:  http.StatusUnauthorized,
			Err:         fmt.Errorf("authorization decoding error: %w", err),
		}
	}
	validAuth := fmt.Sprintf("%s:%s", domainConfig.Domain, domainConfig.Secret)
	if string(auth) != validAuth {
		return app.HandlerError{
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
			return app.HandlerError{
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

	respIP := ""
	if domainConfig.AllowClientIPChoice {
		respIP = updateRequest.MyIP
	} else if updateARecordValue != "" {
		respIP = updateARecordValue
	} else if updateAAAARecordValue != "" {
		respIP = updateAAAARecordValue
	}

	w.WriteHeader(http.StatusOK)
	_, err = fmt.Fprintf(w, "good %s", respIP)
	return err
}

// remoteAddr returns the client IP address, taking into account the x-forwarded-for header.
// It parses the IP, and also returns the version of the client IP.
// If the client IP can't be parsed, it returns only an error.
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

// ipVersion returns the version of the given IP address string, or an error
// if the address cannot be parsed.
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

func performUpdate(e *app.Env, c app.DomainConfig, recordType string, value string) error {
	if e.UpdateCache.Get(c.Domain, recordType) == value {
		log.Printf("cache indicates that %s record for %s is up to date", recordType, c.Domain)
		return nil
	}

	parts := strings.Split(c.Domain, ".")
	if len(parts) < 2 {
		return app.HandlerError{
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
		return app.HandlerError{
			StatusCode: http.StatusInternalServerError,
			Err:        err,
		}
	}

	e.UpdateCache.Set(c.Domain, recordType, value)
	return nil
}