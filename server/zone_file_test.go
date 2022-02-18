package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withBind9TestServer(ctx context.Context, callback func(resolver *net.Resolver)) error {
	namedPath, err := filepath.Abs("test_data/named.conf")
	if err != nil {
		return err
	}
	zonefilePath, err := filepath.Abs("test_data/zonefile")
	if err != nil {
		return err
	}
	binds := []string{
		fmt.Sprintf("%s:/etc/bind/named.conf", namedPath),
		fmt.Sprintf("%s:/etc/bind/zonefile", zonefilePath),
	}

	return withDockerContainer(ctx, "internetsystemsconsortium/bind9:9.16", "0:53/udp", binds, func(ctx context.Context, c *client.Client, containerId string) error {
		logs, err := c.ContainerLogs(ctx, containerId, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			return err
		}
		defer logs.Close()
		logsScanner := bufio.NewScanner(logs)
		for logsScanner.Scan() {
			if logsScanner.Err() != nil {
				return logsScanner.Err()
			}
			fmt.Println(logsScanner.Text())
			if strings.HasSuffix(logsScanner.Text(), " running") {
				break
			}
		}

		dnsPort, err := getContainerPort(ctx, c, containerId, "udp", "53")
		if err != nil {
			return err
		}

		bindResolver := buildLocalhostResolver(dnsPort)

		callback(bindResolver)

		return nil
	})
}

func TestMatchesBind(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver, nameserver string) {

		zoneFile, err := os.Open("test_data/zonefile")
		assert.NoError(t, err)
		defer zoneFile.Close()
		response, err := apiClient.PostZoneWithBody(ctx, "text/plain", zoneFile)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, response.StatusCode)

		err = withBind9TestServer(ctx, func(bindResolver *net.Resolver) {
			// TODO: Once the dns query handler implements RFC1034, this test should systematically compare
			// bind and the dns server for all combinations of domain name from the test zone file and record types
			for _, domain := range []string{"home.zonetransfer.me", "invalid.zonetransfer.me", "zonetransfer.me"} {
				expectedHost, expectedErr := bindResolver.LookupHost(ctx, domain)
				actualHost, actualErr := resolver.LookupHost(ctx, domain)

				assert.Equal(t, expectedErr, actualErr, "err expected != actual for domain=%s", domain)
				assert.Equal(t, expectedHost, actualHost, "host expected != actual for domain=%s", domain)
			}
		})
		assert.NoError(t, err)
	})
}

func TestInvalidZoneFile_400s(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, c *Client, resolver *net.Resolver, nameserver string) {
		body, err := c.PostZoneWithBody(ctx, "text/plain", strings.NewReader("not\nso\nvalid"))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, body.StatusCode)
	})
}
