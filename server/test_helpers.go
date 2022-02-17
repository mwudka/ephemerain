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
	"github.com/teris-io/shortid"
	"net"
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

	image := "redis:6.2-alpine"
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

	_, m, _ := nat.ParsePortSpecs([]string{"0:6379"})
	shortId, err := shortid.Generate()
	if err != nil {
		return err
	}
	containerName := "domaintest_redis_" + shortId
	create, err := client.ContainerCreate(context.Background(), &container.Config{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		OpenStdin:    true,
		Image:        image,
	}, &container.HostConfig{
		PortBindings: m,
		AutoRemove:   true,
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

func withServer(ctx context.Context, config EphemerainConfig, callback func(client *Client, resolver *net.Resolver, nameserver string)) error {
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

	nameserver := fmt.Sprintf("127.0.0.1:%d", dnsServerPort)
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, network, nameserver)
		},
	}

	apiClient, err := NewClient(fmt.Sprintf("http://localhost:%d/v1", httpServerPort))
	if err != nil {
		return err
	}

	callback(apiClient, resolver, nameserver)

	return nil
}

func runIntegrationTest(t *testing.T, callback func(context.Context, *Client, *net.Resolver, string)) {
	ctx := context.Background()
	err := withRedisTestServer(ctx, func(redisPort int) {
		config := EphemerainConfig{
			JSONLogs:     false,
			RedisAddress: fmt.Sprintf("localhost:%d", redisPort),
		}
		err := withServer(ctx, config, func(apiClient *Client, resolver *net.Resolver, nameserver string) {
			callback(ctx, apiClient, resolver, nameserver)
		})
		if err != nil {
			t.Fatalf("Error running test server: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("Error running redis test server: %v", err)
	}
}
