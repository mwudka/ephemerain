package main

import (
	"flag"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hashicorp/go-hclog"
	"github.com/miekg/dns"
	"github.com/teris-io/shortid"
	"golang.org/x/net/context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func handleIPQuery(registrar Registrar) func(w dns.ResponseWriter, r *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		ctx := hclog.WithContext(context.Background(), hclog.L(), "request_id", r.Id)
		logger := hclog.FromContext(ctx)
		logger.Info("Received DNS message", "message", r.String())

		// TODO: Probably split it into its own method
		if r.Opcode == dns.OpcodeUpdate {
			logger.Info("Performing update")

			for _, ns := range r.Ns {
				fqdn := Domain(ns.Header().Name)
				switch ns.Header().Rrtype {
				case dns.TypeA:
					ip := ns.(*dns.A).A.String()
					registrar.SetRecord(ctx, fqdn, RecordTypeA, ip)
				case dns.TypeCNAME:
					target := ns.(*dns.CNAME).Target
					registrar.SetRecord(ctx, fqdn, RecordTypeCNAME, target)
				case dns.TypeTXT:
					// TODO: Support multiple values
					// TODO: Handle deletion
					txt := ns.(*dns.TXT).Txt
					if len(txt) > 0 {
						values := txt[0]
						registrar.SetRecord(ctx, fqdn, RecordTypeTXT, values)
					}
				}
			}

			// TODO: What is the return message supposed to say?
			m := new(dns.Msg)
			m.SetReply(r)
			m.Compress = false
			w.WriteMsg(m)
			return
		}

		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false
		m.Authoritative = true
		m.RecursionAvailable = false

		dom := Domain(r.Question[0].Name)

		switch r.Question[0].Qtype {
		case dns.TypeNS:
			// TODO: Should this, like, recurse or something? Letsencrypt always checks for NS records on random
			// subdomains
			m.Rcode = dns.RcodeSuccess
			rr := &dns.NS{
				Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 60},
				Ns:  "ns1.ephemerain.com.",
			}
			m.Answer = append(m.Answer, rr)
		case dns.TypeSOA:
			m.Rcode = dns.RcodeSuccess
			// TODO: What are these supposed to be?
			rr := &dns.SOA{
				Hdr:     dns.RR_Header{Name: string(dom), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60},
				Ns:      "ns-822.awsdns-38.net.",
				Mbox:    "awsdns-hostmaster.amazon.com.",
				Serial:  1,
				Refresh: 7200,
				Retry:   900,
				Expire:  1209600,
				Minttl:  86400,
			}
			m.Answer = append(m.Answer, rr)
		case dns.TypeCNAME:
			value, err := registrar.GetRecord(ctx, dom, "CNAME")
			if err != nil {
				logger.Error("Error getting CNAME record", "fqdn", dom, "error", err)
			} else {
				rr := &dns.CNAME{
					Hdr:    dns.RR_Header{Name: string(dom), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
					Target: value,
				}
				m.Answer = append(m.Answer, rr)
			}
		case dns.TypeTXT:
			value, err := registrar.GetRecord(ctx, dom, "TXT")
			if err != nil {
				logger.Error("Error getting TXT record", "fqdn", dom, "error", err)

				// TODO: This seems really weird. Is it correct to fallback to CNAME if TXT isn't present?
				// registry.terraform.io seems to do it and AWS ACM validation queries TXT records even though
				// it says to create CNAME records, so maybe???
				// TODO: Cleanup duplication
				value, err := registrar.GetRecord(ctx, dom, "CNAME")
				if err != nil {
					logger.Error("Error getting CNAME record", "fqdn", dom, "error", err)
				} else {
					rr := &dns.CNAME{
						Hdr:    dns.RR_Header{Name: string(dom), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
						Target: value,
					}
					m.Answer = append(m.Answer, rr)
				}
			} else {
				rr := &dns.TXT{
					Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60},
					Txt: []string{value},
				}
				m.Answer = append(m.Answer, rr)
			}
		case dns.TypeA:
			ipv4QueryRegex := regexp.MustCompile(`(?P<ipv4>(?:\d+\D){3}\d+)\.ip\.[^.]+\.[^.]+\.`)
			submatch := ipv4QueryRegex.FindStringSubmatch(string(dom))
			if len(submatch) == 2 {
				requestedIPv4 := submatch[1]

				normalizedIPv4 := strings.Join(regexp.MustCompile(`\D`).Split(requestedIPv4, 4), ".")

				rr := &dns.A{
					Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
					A:   net.ParseIP(normalizedIPv4),
				}
				m.Answer = append(m.Answer, rr)
			} else {
				value, err := registrar.GetRecord(ctx, dom, "A")
				if err != nil {
					logger.Error("Error getting A record", "fqdn", dom, "error", err)
				} else {
					rr := &dns.A{
						Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
						A:   net.ParseIP(value),
					}
					m.Answer = append(m.Answer, rr)
				}
			}
		}

		logger.Info("Writing response", "message", m.String())
		err := w.WriteMsg(m)
		if err != nil {
			logger.Error("Failed to write DNS response", "error", err.Error())
		}
	}
}

func serveDNS() {
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
	if err := server.ListenAndServe(); err != nil {
		hclog.L().Error("Failed to start DNS server", "error", err.Error())
		// TODO: What is the right way to handle server startup failure? If DNS fails but HTTP works it might be
		// nice to at least serve the HTTP component. Maybe this is a signal that they should be different containers?
		panic(err)
	}
}

func serveAPI(registrar Registrar) {
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
	if err := http.ListenAndServe(":80", r); err != nil {
		hclog.L().Error("Error starting API server", "error", err)
		// TODO: What is the right way to handle server startup failure? If DNS fails but HTTP works it might be
		// nice to at least serve the HTTP component. Maybe this is a signal that they should be different containers?
		panic(err)
	}
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
	go serveDNS()
	go serveAPI(registrar)
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	hclog.L().Info("Received signal; stopping", "signal", s.String())
}
