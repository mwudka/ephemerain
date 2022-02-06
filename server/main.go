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
		fmt.Printf("Got message %s\n", r)

		// TODO: Probably split it into its own method
		if r.Opcode == dns.OpcodeUpdate {
			fmt.Printf("Update request received\n")

			ns := r.Ns[0]
			fqdn := Domain(ns.Header().Name)
			switch ns.Header().Rrtype {
			case dns.TypeA:
				ip := ns.(*dns.A).A.String()
				registrar.SetRecord(fqdn, RecordTypeA, ip)
			case dns.TypeCNAME:
				target := ns.(*dns.CNAME).Target
				registrar.SetRecord(fqdn, RecordTypeCNAME, target)
			case dns.TypeTXT:
				// TODO: Support multiple values
				// TODO: Handle deletion
				txt := ns.(*dns.TXT).Txt
				if len(txt) > 0 {
					values := txt[0]
					registrar.SetRecord(fqdn, RecordTypeTXT, values)
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

		dom := Domain(r.Question[0].Name)

		switch r.Question[0].Qtype {
		case dns.TypeCNAME:
			value, err := registrar.GetRecord(dom, "CNAME")
			if err != nil {
				fmt.Printf("Error getting CNAME record for %s: %v\n", dom, err)
			} else {
				rr := &dns.CNAME{
					Hdr:    dns.RR_Header{Name: string(dom), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
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
					Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60},
					Txt: []string{value},
				}
				m.Answer = append(m.Answer, rr)
			}
		case dns.TypeA:
			ipv4QueryRegex := regexp.MustCompile(`(?P<ipv4>(?:\d+\D){3}\d+)\.ip\.[^.]+\.[^.]+\.`)
			submatch := ipv4QueryRegex.FindStringSubmatch(string(dom))
			fmt.Printf("Found submatch %v for %s\n", submatch, dom)
			if len(submatch) == 2 {
				requestedIPv4 := submatch[1]
				fmt.Printf("Raw requested IPv4 is %s\n", requestedIPv4)

				normalizedIPv4 := strings.Join(regexp.MustCompile(`\D`).Split(requestedIPv4, 4), ".")

				rr := &dns.A{
					Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
					A:   net.ParseIP(normalizedIPv4),
				}
				m.Answer = append(m.Answer, rr)
			} else {
				value, err := registrar.GetRecord(dom, "A")
				if err != nil {
					fmt.Printf("Error getting A record for %s: %v\n", dom, err)
				} else {
					rr := &dns.A{
						Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
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
	// Same as the default accept function, but allows update messages
	acceptFunc := func(dh dns.Header) dns.MsgAcceptAction {
		if isResponse := dh.Bits&/*dns._QR*/(1 << 15) != 0; isResponse {
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
		fmt.Printf("Failed to setup the dns: %v\n", err.Error())
		// TODO: What is the right way to handle server startup failure? If DNS fails but HTTP works it might be
		// nice to at least serve the HTTP component. Maybe this is a signal that they should be different containers?
		panic(err)
	}
}

func serveAPI(registrar Registrar) {
	r := chi.NewRouter()

	// TODO: Structured logging
	// TODO: Ratelimiting
	r.Use(middleware.Logger)

	api := DomainAPIImpl{registrar: registrar}
	r.Mount("/v1", Handler(&api))
	if err := http.ListenAndServe(":80", r); err != nil {
		fmt.Printf("Error starting API server: %v\n", err)
		// TODO: What is the right way to handle server startup failure? If DNS fails but HTTP works it might be
		// nice to at least serve the HTTP component. Maybe this is a signal that they should be different containers?
		panic(err)
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
