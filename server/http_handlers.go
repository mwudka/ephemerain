package main

import (
	"encoding/json"
	"github.com/hashicorp/go-hclog"
	"net/http"
)

type DomainAPIImpl struct {
	registrar Registrar
}

func (d DomainAPIImpl) GetDomain(w http.ResponseWriter, r *http.Request, domain Domain, recordType RecordType) {
	logger := hclog.FromContext(r.Context())
	record, err := d.registrar.GetRecord(r.Context(), domain, recordType)
	if err != nil {
		logger.Info("Error getting record from registrar", "error", err)
		w.WriteHeader(http.StatusNotFound)
	} else {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&RecordValue{Value: &record}); err != nil {
			logger.Info("Error getting record from registrar", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func (d DomainAPIImpl) PutDomain(w http.ResponseWriter, r *http.Request, domain Domain, recordType RecordType) {
	logger := hclog.FromContext(r.Context())
	var body PutDomainJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		logger.Error("Malformed request", "error", err)

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: DNS server library might have handy ways to validate different record values
	// TODO: Validate the record types. FQDN for CNAME, IP for A, etc
	// TODO: Validate lengths

	logger.Info("Setting record", "domain", domain, "type", recordType, "value", *body.Value)
	err := d.registrar.SetRecord(r.Context(), domain, recordType, *body.Value)
	if err != nil {
		logger.Error("Error from registrar when setting record", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
