package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func withRedisTestServer(ctx context.Context, callback func(int)) error {
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer client.Close()

	_, m, _ := nat.ParsePortSpecs([]string{"0:6379"})
	create, err := client.ContainerCreate(context.Background(), &container.Config{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		OpenStdin:    true,
		Image:        "redis:6.2-alpine",
	}, &container.HostConfig{
		PortBindings: m,
		AutoRemove:   true,
	}, &network.NetworkingConfig{}, nil, "testing")
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
	port, _ := nat.NewPort("tcp", "6379")
	redisPortRaw := inspect.NetworkSettings.Ports[port][0].HostPort

	redisPort, err := strconv.Atoi(redisPortRaw)
	if err != nil {
		return err
	}

	defer func() {
		duration, _ := time.ParseDuration("5s")
		_ = client.ContainerStop(ctx, create.ID, &duration)
	}()

	callback(redisPort)

	return nil
}

func withServer(ctx context.Context, config EphemerainConfig, callback func(client *Client, resolver *net.Resolver)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dnsListener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	dnsServerPort := dnsListener.LocalAddr().(*net.UDPAddr).Port
	config.DNSListener = dnsListener

	httpListener, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	httpServerPort := httpListener.Addr().(*net.TCPAddr).Port
	config.HTTPListener = httpListener

	runServer(ctx, config)

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, network, fmt.Sprintf("127.0.0.1:%d", dnsServerPort))
		},
	}

	apiClient, err := NewClient(fmt.Sprintf("http://localhost:%d/v1", httpServerPort))
	if err != nil {
		return err
	}

	callback(apiClient, resolver)

	return nil
}

func runIntegrationTest(t* testing.T, callback func(context.Context, *Client, *net.Resolver)) {
	ctx := context.Background()
	err := withRedisTestServer(ctx, func(redisPort int) {
		config := EphemerainConfig{
			JSONLogs:     false,
			RedisAddress: fmt.Sprintf("localhost:%d", redisPort),
		}
		err := withServer(ctx, config, func(apiClient *Client, resolver *net.Resolver) {
			callback(ctx, apiClient, resolver)
		})
		if err != nil {
			t.Fatalf("Error running test server: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("Error running redis test server: %v", err)
	}
}

func TestE2E(t *testing.T) {
	runIntegrationTest(t, func(ctx context.Context, apiClient *Client, resolver *net.Resolver) {
		expectedHost := []string{"1.2.3.4"}
		domain := "testingsub.testingdomain.com."

		response, err := apiClient.PutDomain(ctx, Domain(domain), RecordTypeA, PutDomainJSONRequestBody{Value: &expectedHost[0]})
		if err != nil {
			t.Errorf("Error setting domain: %v", err)
		}
		if http.StatusNoContent != response.StatusCode {
			t.Errorf("Error setting domain: %v", response)
		}

		host, err := resolver.LookupHost(context.Background(), domain)
		if err != nil {
			t.Errorf("Error looking up host: %s", err)
		}

		if len(host) != 1 || host[0] != expectedHost[0] {
			t.Errorf("Incorrect response. Expected %s; got %s", expectedHost, host)
		}
	})
}
