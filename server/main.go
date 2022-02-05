package main

import (
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	dom := r.Question[0].Name

	switch r.Question[0].Qtype {
	default:
		fallthrough
	case dns.TypeAAAA:
		rr := &dns.AAAA{
			Hdr:  dns.RR_Header{Name: dom, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 0},
			AAAA: net.ParseIP("2606:2800:220:1:248:1893:25c8:1946"),
		}
		m.Answer = append(m.Answer, rr)
	case dns.TypeA:
		rr := &dns.A{
			Hdr: dns.RR_Header{Name: dom, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
			A:   net.ParseIP("93.184.216.34"),
		}
		m.Answer = append(m.Answer, rr)
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
	dns.HandleFunc(".", handleQuery)
	go serve()
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	fmt.Printf("Signal (%s) received, stopping\n", s)
}
