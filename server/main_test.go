package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/providers/dns/rfc2136"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
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

func TestDNS_ReturnsHardcodedNS(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, _ string) {
		ns, err := resolver.LookupNS(ctx, "bam0.com")
		assert.NoError(t, err)

		hosts := make([]string, len(ns))
		for idx, host := range ns {
			hosts[idx] = host.Host
		}
		assert.Equal(t, []string{"ns1.bam0.com."}, hosts)
	})
}

func TestLegoRFC2136(t *testing.T) {
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
		assert.Error(t, err, "Domain name should have been deleted")
		assert.True(t, err.(*net.DNSError).IsNotFound)
		assert.Empty(t, txt)
	})
}

//go:embed test_data/tfc2135.tf
var tfc2135Config []byte

func TestTerraformRFC2135(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, nameserver string) {
		installer := releases.ExactVersion{Product: product.Terraform, Version: version.Must(version.NewVersion("1.1.6"))}
		terraformExecPath, err := installer.Install(ctx)
		assert.NoError(t, err)

		tempDir, err := ioutil.TempDir("", "ephemerain-test")
		defer os.RemoveAll(tempDir)
		assert.NoError(t, err)

		err = ioutil.WriteFile(path.Join(tempDir, "tfc2135.tf"), tfc2135Config, 0644)
		assert.NoError(t, err)

		terraform, err := tfexec.NewTerraform(tempDir, terraformExecPath)
		assert.NoError(t, err)

		err = terraform.Init(ctx, tfexec.Upgrade(true))
		assert.NoError(t, err)

		host, port, err := net.SplitHostPort(nameserver)
		assert.NoError(t, err)
		err = terraform.Apply(ctx, tfexec.Var("server="+host), tfexec.Var("port="+port))
		assert.NoError(t, err)

		addrs, err := resolver.LookupHost(ctx, "a.something.example.com")
		assert.NoError(t, err)
		assert.Equal(t, []string{"1.2.3.4"}, addrs)
	})
}
