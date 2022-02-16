package main

import (
	"context"
	"encoding/json"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/providers/dns/rfc2136"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestSetARecord(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, _ string) {
		expectedHost := []string{"1.2.3.4"}
		domain := "testingsub.testingdomain.com."

		response, err := apiClient.PutDomain(ctx, Domain(domain), RecordTypeA, PutDomainJSONRequestBody{Value: &expectedHost[0]})
		assert.NoError(t, err, "Error setting domain")
		assert.Equal(t, http.StatusNoContent, response.StatusCode, "Error setting domain")

		host, err := resolver.LookupHost(ctx, domain)
		assert.NoError(t, err, "Error looking up host")
		assert.Equal(t, expectedHost, host, "Incorrect response")
	})
}

func TestMissingARecord(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, _ string) {
		domain := "testingsub.testingdomain.com."

		host, err := resolver.LookupHost(ctx, domain)
		assert.Truef(t, err.(*net.DNSError).IsNotFound, "Should be not found")
		assert.Nil(t, host, "No results should be returned")
	})
}

func TestIPSubdomain(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, _ string) {
		domain := "10.20.30.40.ip.testingdomain.com."

		host, err := resolver.LookupHost(ctx, domain)
		assert.NoError(t, err, "Error looking up host")
		assert.Equal(t, []string{"10.20.30.40"}, host, "Incorrect response")
	})
}

func TestAPI_PutDomain_400_IfBadRequest(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, _ string) {
		domain, err := apiClient.PutDomainWithBody(ctx, "foo.com.", RecordTypeA, "application/json", strings.NewReader("not valid json"))
		assert.NoError(t, err, "Error getting domain")
		assert.Equal(t, http.StatusBadRequest, domain.StatusCode)
	})
}

func TestAPI_GetDomain_404_IfNotFound(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, _ string) {
		domain, err := apiClient.GetDomain(ctx, "foo.com.", RecordTypeA)
		assert.NoError(t, err, "Error getting domain")
		assert.Equal(t, http.StatusNotFound, domain.StatusCode)
	})
}

func TestAPI_GetDomain_200_IfFound(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, _ string) {
		records := "2.4.6.8"
		putResponse, err := apiClient.PutDomain(ctx, "foo.com.", RecordTypeA, PutDomainJSONRequestBody{Value: &records})
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, putResponse.StatusCode)

		domain, err := apiClient.GetDomain(ctx, "foo.com.", RecordTypeA)
		assert.NoError(t, err, "Error getting domain")
		defer domain.Body.Close()
		assert.Equal(t, http.StatusOK, domain.StatusCode)
		var response RecordValue
		all, err := io.ReadAll(domain.Body)
		assert.NoError(t, err)
		err = json.Unmarshal(all, &response)
		assert.NoError(t, err)
		assert.Equal(t, records, *response.Value)
	})
}

func TestRFC2136(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, nameserver string) {
		domain := "rfc2136.testing.com"
		keyAuth := "some-key-auth"
		token := "some-token"

		dnsProvider, err := rfc2136.NewDNSProviderConfig(&rfc2136.Config{Nameserver: nameserver})
		assert.NoError(t, err)
		err = dnsProvider.Present(domain, token, keyAuth)
		assert.NoError(t, err)

		record, value := dns01.GetRecord(domain, keyAuth)

		txt, err := resolver.LookupTXT(ctx, record)
		assert.NoError(t, err)
		assert.Equal(t, []string{value}, txt)

		err = dnsProvider.CleanUp(domain, token, keyAuth)
		assert.NoError(t, err)

		txt, err = resolver.LookupTXT(ctx, record)
		assert.NoError(t, err)
		// TODO: Re-add this assert once deleting records is implemented
		//assert.Empty(t, txt)
	})
}
