package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hashicorp/go-hclog"
	"github.com/miekg/dns"
	"github.com/teris-io/shortid"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func serveDNS(ctx context.Context, packetConn net.PacketConn) error {
	logger := hclog.FromContext(ctx)

	// Same as the default accept function, but allows update messages
	acceptFunc := func(dh dns.Header) dns.MsgAcceptAction {
		if isResponse := dh.Bits& /*dns._QR*/ (1<<15) != 0; isResponse {
			return dns.MsgIgnore
		}

		opcode := int(dh.Bits>>11) & 0xF
		if opcode != dns.OpcodeQuery && opcode != dns.OpcodeNotify && opcode != dns.OpcodeUpdate {
			return dns.MsgRejectNotImplemented
		}

		if dh.Qdcount != 1 {
			return dns.MsgReject
		}
		// NOTIFY requests can have a SOA in the ANSWER section. See RFC 1996 Section 3.7 and 3.11.
		if dh.Ancount > 1 {
			return dns.MsgReject
		}
		if dh.Arcount > 2 {
			return dns.MsgReject
		}
		return dns.MsgAccept
	}

	server := &dns.Server{PacketConn: packetConn, TsigSecret: nil, ReusePort: false, MsgAcceptFunc: acceptFunc}
	go func() {
		<-ctx.Done()
		logger.Info("Shutting down DNS server")
		if err := server.ShutdownContext(ctx); err != nil && err != context.Canceled {
			logger.Error("Error shutting down DNS server", "error", err)
		}
	}()

	return server.ActivateAndServe()
}

func serveAPI(ctx context.Context, registrar Registrar, listener net.Listener) error {
	r := chi.NewRouter()

	// TODO: Ratelimiting
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestId, _ := shortid.Generate()
			logger := hclog.FromContext(r.Context()).With("request_id", requestId)

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			t1 := time.Now()
			defer func() {
				logger.Info("HTTP Request", "requestMethod", r.Method, "requestUrl", r.URL.String(), "status", ww.Status(), "latency", time.Since(t1).Seconds(), "protocol", r.Proto)
			}()

			next.ServeHTTP(ww, r.WithContext(hclog.WithContext(r.Context(), logger)))
		})
	})

	api := DomainAPIImpl{registrar: registrar}
	r.Mount("/v1", Handler(&api))

	server := http.Server{Handler: r}

	go func() {
		<-ctx.Done()
		logger := hclog.FromContext(ctx)
		logger.Info("Shutting down HTTP server")
		if err := server.Shutdown(ctx); err != nil && err != context.Canceled {
			logger.Error("Error shutting down HTTP server", "error", err)
		}
	}()

	return server.Serve(listener)
}

type EphemerainConfig struct {
	JSONLogs     bool
	RedisAddress string
	DNSListener  net.PacketConn
	HTTPListener net.Listener
}

func runServer(ctx context.Context, config EphemerainConfig) {
	hclog.DefaultOptions = &hclog.LoggerOptions{JSONFormat: config.JSONLogs}
	hclog.L().Info("Starting up")
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()

	hclog.L().Info(fmt.Sprintf("Using redis address %s", config.RedisAddress))

	registrar := NewRedisRegistrar(config.RedisAddress)

	dns.HandleFunc(".", handleIPQuery(registrar))
	go func() {
		err := serveDNS(ctx, config.DNSListener)
		if err != nil {
			hclog.L().Error("Error starting DNS server", "error", err)
			panic(err)
		}
	}()
	go func() {
		err := serveAPI(ctx, registrar, config.HTTPListener)
		if err != nil && err != http.ErrServerClosed {
			hclog.L().Error("Error starting API server", "error", err)
			panic(err)
		}
	}()
}

func main() {
	redisAddress, redisAddressSet := os.LookupEnv("REDIS_ADDRESS")
	if !redisAddressSet {
		redisAddress = "localhost:6379"
	}
	ctx, cancel := context.WithCancel(context.Background())

	dnsListener, err := net.ListenPacket("udp", "[::]:53")
	if err != nil {
		hclog.L().Error("Error starting DNS listener", "error", err)
		panic(err)
	}

	httpListener, err := net.Listen("tcp", ":80")
	if err != nil {
		hclog.L().Error("Error starting HTTP listener", "error", err)
		panic(err)
	}

	runServer(ctx, EphemerainConfig{
		JSONLogs: strings.ToLower(os.Getenv("LOG_FORMAT")) == "json",
		RedisAddress: redisAddress,
		DNSListener: dnsListener,
		HTTPListener: httpListener,
	})

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	hclog.L().Info("Received signal; stopping", "signal", s.String())
	cancel()
}
