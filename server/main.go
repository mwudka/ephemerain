package main

import (
	"flag"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/miekg/dns"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

func handleIPQuery(registrar Registrar) func(w dns.ResponseWriter, r *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false

		dom := r.Question[0].Name

		switch r.Question[0].Qtype {
		default:
			fallthrough
		case dns.TypeCNAME:
			value, err := registrar.GetRecord(dom, "CNAME")
			if err != nil {
				fmt.Printf("Error getting CNAME record for %s: %v\n", dom, err)
			} else {
				rr := &dns.CNAME{
					Hdr:    dns.RR_Header{Name: dom, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 0},
					Target: value,
				}
				m.Answer = append(m.Answer, rr)
			}
		case dns.TypeTXT:
			value, err := registrar.GetRecord(dom, "TXT")
			if err != nil {
				fmt.Printf("Error getting CNAME record for %s: %v\n", dom, err)
			} else {
				rr := &dns.TXT{
					Hdr: dns.RR_Header{Name: dom, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
					Txt: []string{value},
				}
				m.Answer = append(m.Answer, rr)
			}
		case dns.TypeA:
			ipv4QueryRegex := regexp.MustCompile(`(?P<ipv4>(?:\d+\D){3}\d+)\.ip\.[^.]+\.[^.]+\.`)
			submatch := ipv4QueryRegex.FindStringSubmatch(dom)
			fmt.Printf("Found submatch %v for %s\n", submatch, dom)
			if len(submatch) == 2 {
				requestedIPv4 := submatch[1]
				fmt.Printf("Raw requested IPv4 is %s\n", requestedIPv4)

				normalizedIPv4 := strings.Join(regexp.MustCompile(`\D`).Split(requestedIPv4, 4), ".")

				rr := &dns.A{
					Hdr: dns.RR_Header{Name: dom, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
					A:   net.ParseIP(normalizedIPv4),
				}
				m.Answer = append(m.Answer, rr)
			} else {
				value, err := registrar.GetRecord(dom, "A")
				if err != nil {
					fmt.Printf("Error getting A record for %s: %v\n", dom, err)
				} else {
					rr := &dns.A{
						Hdr: dns.RR_Header{Name: dom, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
						A:   net.ParseIP(value),
					}
					m.Answer = append(m.Answer, rr)
				}
			}
		}

		fmt.Printf("%v\n", m.String())
		err := w.WriteMsg(m)
		if err != nil {
			fmt.Printf("Failed to write DNS response: %v\n", err.Error())
		}
	}
}

func serveDNS() {
	server := &dns.Server{Addr: "[::]:53", Net: "udp", TsigSecret: nil, ReusePort: false}
	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("Failed to setup the dns: %v\n", err.Error())
	}
}

func serveAPI(registrar Registrar) {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	api := DomainAPIImpl{registrar: registrar}
	r.Mount("/v1", Handler(&api))
	if err := http.ListenAndServe(":80", r); err != nil {
		fmt.Printf("Error starting API server: %v\n", err)
	}
}

func main() {
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()

	registrar := InMemoryRegistrar{
		records: map[string]string{},
	}

	dns.HandleFunc(".", handleIPQuery(registrar))
	go serveDNS()
	go serveAPI(registrar)
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	fmt.Printf("Signal (%s) received, stopping\n", s)
}
