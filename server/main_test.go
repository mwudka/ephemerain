package main

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestSetARecord(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver) {
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
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver) {
		domain := "testingsub.testingdomain.com."

		host, err := resolver.LookupHost(ctx, domain)
		assert.Truef(t, err.(*net.DNSError).IsNotFound, "Should be not found")
		assert.Nil(t, host, "No results should be returned")
	})
}

func TestIPSubdomain(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver) {
		domain := "10.20.30.40.ip.testingdomain.com."

		host, err := resolver.LookupHost(ctx, domain)
		assert.NoError(t, err, "Error looking up host")
		assert.Equal(t, []string{"10.20.30.40"}, host, "Incorrect response")
	})
}

func TestAPI_PutDomain_400_IfBadRequest(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver) {
		domain, err := apiClient.PutDomainWithBody(ctx, "foo.com.", RecordTypeA, "application/json", strings.NewReader("not valid json"))
		assert.NoError(t, err, "Error getting domain")
		assert.Equal(t, http.StatusBadRequest, domain.StatusCode)
	})
}

func TestAPI_GetDomain_404_IfNotFound(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver) {
		domain, err := apiClient.GetDomain(ctx, "foo.com.", RecordTypeA)
		assert.NoError(t, err, "Error getting domain")
		assert.Equal(t, http.StatusNotFound, domain.StatusCode)
	})
}

func TestAPI_GetDomain_200_IfFound(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver) {
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
