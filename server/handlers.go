package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type DomainAPIImpl struct {
	registrar Registrar
}

func (d DomainAPIImpl) GetDomain(w http.ResponseWriter, r *http.Request, domain Domain, recordType RecordType) {
	record, err := d.registrar.GetRecord(domain, recordType)
	if err != nil {
		fmt.Printf("Error getting record %v\n", err)
		w.WriteHeader(404)
	} else {
		w.WriteHeader(200)
		if err := json.NewEncoder(w).Encode(&RecordValue{Value: &record}); err != nil {
			// TODO: Handle error
			panic(err)
		}
	}
}

func (d DomainAPIImpl) PutDomain(w http.ResponseWriter, r *http.Request, domain Domain, recordType RecordType) {
	var body PutDomainJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// TODO: Better error handling
		panic(err)
	}

	// TODO: DNS server library might have handy ways to validate different record values
	// TODO: Validate the record types. FQDN for CNAME, IP for A, etc
	// TODO: Validate lengths

	fmt.Printf("%s %s\n", recordType, *body.Value)
	d.registrar.SetRecord(domain, recordType, *body.Value)

	w.WriteHeader(204)
}
