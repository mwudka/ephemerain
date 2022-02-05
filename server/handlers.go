package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type DomainAPIImpl struct {
	registrar Registrar
}

func (d DomainAPIImpl) PutDomain(w http.ResponseWriter, r *http.Request, domain string) {
	var body PutDomainJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// TODO: Better error handling
		panic(err)
	}

	fmt.Printf("%s %s\n", *body.Type, *body.Value)
	d.registrar.SetRecord(domain, *body.Type, *body.Value)

	w.WriteHeader(200)
}
