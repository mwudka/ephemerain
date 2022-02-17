package main

import (
	"encoding/json"
	"github.com/hashicorp/go-hclog"
	"github.com/wpalmer/gozone"
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

// TODO: Maybe this should be scoped to a domain?
func (d DomainAPIImpl) PostZone(w http.ResponseWriter, r *http.Request) {
	logger := hclog.FromContext(r.Context())
	var record gozone.Record
	scanner := gozone.NewScanner(r.Body)
	defer r.Body.Close()
	for {
		err := scanner.Next(&record)
		if err == nil {
			logger.Info("Parsed zone record", "record", record)

			if record.Type == gozone.RecordType_A {
				err := d.registrar.SetRecord(r.Context(), Domain(record.DomainName), RecordTypeA, record.Data[0])
				if err != nil {
					logger.Warn("Error setting record", "error", err)
				}
			}

		} else if err.Error() == "EOF" {
			logger.Info("Finishing processing uploaded zone")
			w.WriteHeader(http.StatusNoContent)
			return
		} else {
			logger.Info("Attempted to post invalid zone", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

	}
}
