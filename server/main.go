package main

import (
	"flag"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hashicorp/go-hclog"
	"github.com/miekg/dns"
	"github.com/teris-io/shortid"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func serveDNS() error {
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
	server := &dns.Server{Addr: "[::]:53", Net: "udp", TsigSecret: nil, ReusePort: false, MsgAcceptFunc: acceptFunc}
	return server.ListenAndServe()
}

func serveAPI(registrar Registrar) error {
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
	return http.ListenAndServe(":80", r)
}

func main() {
	hclog.DefaultOptions = &hclog.LoggerOptions{JSONFormat: strings.ToLower(os.Getenv("LOG_FORMAT")) == "json"}
	hclog.L().Info("Starting up")
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()

	redisAddress, redisAddressSet := os.LookupEnv("REDIS_ADDRESS")
	if !redisAddressSet {
		redisAddress = "localhost:6379"
		hclog.L().Info(fmt.Sprintf("Using default redis address %s", redisAddress))
	}

	registrar := NewRedisRegistrar(redisAddress)

	dns.HandleFunc(".", handleIPQuery(registrar))
	go func() {
		err := serveDNS()
		if err != nil {
			hclog.L().Error("Error starting DNS server", "error", err)
			panic(err)
		}
	}()
	go func() {
		err := serveAPI(registrar)
		if err != nil {
			hclog.L().Error("Error starting API server", "error", err)
			panic(err)
		}
	}()
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	hclog.L().Info("Received signal; stopping", "signal", s.String())
}
