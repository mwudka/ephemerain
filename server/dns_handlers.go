package main

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"github.com/miekg/dns"
	"net"
	"regexp"
	"strings"
)

func handleIPQuery(registrar Registrar) func(w dns.ResponseWriter, r *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		ctx := hclog.WithContext(context.Background(), hclog.L(), "request_id", r.Id)
		logger := hclog.FromContext(ctx)
		logger.Info("Received DNS message", "message", r.String())

		// TODO: Probably split it into its own method
		if r.Opcode == dns.OpcodeUpdate {
			logger.Info("Performing update")

			var err error = nil
			for _, ns := range r.Ns {
				fqdn := Domain(ns.Header().Name)
				switch ns.Header().Rrtype {
				case dns.TypeA:
					ip := ns.(*dns.A).A.String()
					err = registrar.SetRecord(ctx, fqdn, RecordTypeA, ip)
				case dns.TypeCNAME:
					target := ns.(*dns.CNAME).Target
					err = registrar.SetRecord(ctx, fqdn, RecordTypeCNAME, target)
				case dns.TypeTXT:
					// TODO: Support multiple values
					// TODO: Handle deletion
					txt := ns.(*dns.TXT).Txt
					if len(txt) > 0 {
						values := txt[0]
						err = registrar.SetRecord(ctx, fqdn, RecordTypeTXT, values)
					}
				}
			}

			// TODO: What is the return message supposed to say?
			m := new(dns.Msg)
			if err != nil {
				m.Rcode = dns.RcodeServerFailure
			}
			m.SetReply(r)
			m.Compress = false
			logger.Info("Sending response message", "message", m.String())
			if err := w.WriteMsg(m); err != nil {
				logger.Error("Error sending response message", "error", err)
			}

			return
		}

		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false
		m.Authoritative = true
		m.RecursionAvailable = false

		dom := Domain(r.Question[0].Name)

		switch r.Question[0].Qtype {
		default:
			m.Rcode = dns.RcodeNameError
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
				m.Rcode = dns.RcodeNameError
			} else {
				m.Rcode = dns.RcodeSuccess
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
					m.Rcode = dns.RcodeNameError
				} else {
					m.Rcode = dns.RcodeSuccess
					rr := &dns.CNAME{
						Hdr:    dns.RR_Header{Name: string(dom), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
						Target: value,
					}
					m.Answer = append(m.Answer, rr)
				}
			} else {
				m.Rcode = dns.RcodeSuccess
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

				m.Rcode = dns.RcodeSuccess
				rr := &dns.A{
					Hdr: dns.RR_Header{Name: string(dom), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
					A:   net.ParseIP(normalizedIPv4),
				}
				m.Answer = append(m.Answer, rr)
			} else {
				value, err := registrar.GetRecord(ctx, dom, "A")
				if err != nil {
					logger.Error("Error getting A record", "fqdn", dom, "error", err)
					m.Rcode = dns.RcodeNameError
				} else {
					m.Rcode = dns.RcodeSuccess
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
