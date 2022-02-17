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
	"github.com/hashicorp/go-hclog"
	"github.com/teris-io/shortid"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

func getContainerPort(ctx context.Context, client *client.Client, containerId string, protocol string, port string) (int, error) {
	inspect, err := client.ContainerInspect(ctx, containerId)
	if err != nil {
		return 0, err
	}
	parsedPort, err := nat.NewPort(protocol, port)
	if err != nil {
		return 0, err
	}
	redisPortRaw := inspect.NetworkSettings.Ports[parsedPort][0].HostPort

	return strconv.Atoi(redisPortRaw)
}

var pulledImages sync.Map

func ensureImagePresent(ctx context.Context, client *client.Client, image string) error {
	logger := hclog.FromContext(ctx).With("image", image)

	_, loaded := pulledImages.LoadOrStore(image, true)
	if loaded {
		logger.Info("Image already pulled locally")
		// TODO: If there are multiple tests running in parallel, it's possible a different thread has started pulling
		// the image but hasn't finished yet. If that's the case, this function will return even though the image
		// isn't actually present yet.
	} else {
		logger.Info("Ensuring image is pulled locally")
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
			logger.Info(message.Status)
			if message.ID != "" {
				logger.Info(message.ID)
			}
		}
	}

	return nil
}

func withDockerContainer(ctx context.Context, image string, portSpec string, binds []string, callback func(context.Context, *client.Client, string) error) error {
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer client.Close()

	err = ensureImagePresent(ctx, client, image)
	if err != nil {
		return err
	}

	_, m, _ := nat.ParsePortSpecs([]string{portSpec})
	shortId, err := shortid.Generate()
	if err != nil {
		return err
	}
	containerName := "domaintest_" + shortId
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
		Binds:        binds,
	}, &network.NetworkingConfig{}, nil, containerName)
	if err != nil {
		return err
	}

	err = client.ContainerStart(ctx, create.ID, types.ContainerStartOptions{})
	if err != nil {
		return err
	}

	defer func() {
		duration, _ := time.ParseDuration("5s")
		_ = client.ContainerStop(ctx, create.ID, &duration)
	}()

	return callback(ctx, client, create.ID)
}

func withRedisTestServer(ctx context.Context, callback func(int)) error {
	return withDockerContainer(ctx, "redis:6.2-alpine", "0:6379", nil, func(ctx context.Context, client *client.Client, containerId string) error {
		redisPort, err := getContainerPort(ctx, client, containerId, "tcp", "6379")
		if err != nil {
			return err
		}

		callback(redisPort)

		return nil
	})
}

func buildLocalhostResolver(port int) *net.Resolver {
	nameserver := fmt.Sprintf("127.0.0.1:%d", port)
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, network, nameserver)
		},
	}
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
	resolver := buildLocalhostResolver(dnsServerPort)

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
