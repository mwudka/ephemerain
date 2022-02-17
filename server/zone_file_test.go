package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/teris-io/shortid"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func withBind9TestServer(ctx context.Context, callback func(resolver *net.Resolver)) error {
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer client.Close()

	image := "internetsystemsconsortium/bind9:9.16"
	pull, err := client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(pull)
	defer pull.Close()
	for scanner.Scan() {
		if scanner.Err() != nil {
			return err
		}
		var message jsonmessage.JSONMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			return err
		}
		fmt.Print(message.Status)
		if message.ID != "" {
			fmt.Print(message.ID)
		}
		fmt.Println()
	}

	_, m, _ := nat.ParsePortSpecs([]string{"0:53/udp"})
	shortId, err := shortid.Generate()
	if err != nil {
		return err
	}
	containerName := "domaintest_bind9_" + shortId
	namedPath, err := filepath.Abs("test_data/named.conf")
	if err != nil {
		return err
	}
	zonefilePath, err := filepath.Abs("test_data/zonefile")
	if err != nil {
		return err
	}
	create, err := client.ContainerCreate(context.Background(), &container.Config{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		OpenStdin:    true,
		Image:        image,
		Volumes:      map[string]struct{}{},
	}, &container.HostConfig{
		PortBindings: m,
		AutoRemove:   true,
		Binds: []string{
			fmt.Sprintf("%s:/etc/bind/named.conf", namedPath),
			fmt.Sprintf("%s:/etc/bind/zonefile", zonefilePath),
		},
	}, &network.NetworkingConfig{}, nil, containerName)
	if err != nil {
		return err
	}

	err = client.ContainerStart(ctx, create.ID, types.ContainerStartOptions{})
	if err != nil {
		return err
	}

	inspect, err := client.ContainerInspect(ctx, create.ID)
	if err != nil {
		return err
	}
	port, _ := nat.NewPort("udp", "53")
	dnsPortRaw := inspect.NetworkSettings.Ports[port][0].HostPort

	logs, err := client.ContainerLogs(ctx, create.ID, types.ContainerLogsOptions{
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
		if strings.HasSuffix(logsScanner.Text(), " running") {
			break
		}
	}

	dnsPort, err := strconv.Atoi(dnsPortRaw)
	if err != nil {
		return err
	}

	defer func() {
		duration, _ := time.ParseDuration("5s")
		_ = client.ContainerStop(ctx, create.ID, &duration)
	}()

	bindResolver := buildLocalhostResolver(dnsPort)

	callback(bindResolver)

	return nil
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
			for _, domain := range []string{"home.zonetransfer.me", "invalid.zonetransfer.me", "zonetransfer.me"} {
				expectedHost, expectedErr := bindResolver.LookupHost(ctx, domain)
				actualHost, actualErr := resolver.LookupHost(ctx, domain)

				assert.Equal(t, expectedErr, actualErr, "err expected != actual", domain)
				assert.Equal(t, expectedHost, actualHost, "host expected != actual", domain)
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
