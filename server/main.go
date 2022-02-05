package main

import (
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

func handleIPQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	dom := r.Question[0].Name

	switch r.Question[0].Qtype {
	default:
		fallthrough
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
		}
	}

	fmt.Printf("%v\n", m.String())
	err := w.WriteMsg(m)
	if err != nil {
		fmt.Printf("Failed to write DNS response: %v\n", err.Error())
	}
}

func serve() {
	server := &dns.Server{Addr: "[::]:53", Net: "udp", TsigSecret: nil, ReusePort: false}
	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("Failed to setup the dns: %v\n", err.Error())
	}
}

func main() {
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()
	dns.HandleFunc(".", handleIPQuery)
	go serve()
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	fmt.Printf("Signal (%s) received, stopping\n", s)
}
