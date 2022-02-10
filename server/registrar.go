package main

import (
	"errors"
	"github.com/hashicorp/go-hclog"
	"golang.org/x/net/context"
	"strings"
)

type Registrar interface {
	SetRecord(ctx context.Context, fqdn Domain, recordType RecordType, value string) error
	GetRecord(ctx context.Context, fqdn Domain, recordType RecordType) (string, error)
}

type InMemoryRegistrar struct {
	records map[string]string
}

func (i InMemoryRegistrar) SetRecord(ctx context.Context, fqdn Domain, recordType RecordType, value string) error {
	// TODO: Move to a logging middleware after registrar middlewares exist
	hclog.FromContext(ctx).Named("mem_registrar").Trace("Setting key", "fqdn", fqdn, "type", recordType, "value", value)
	i.records[strings.ToLower(string(fqdn))+":"+string(recordType)] = value
	return nil
}

func (i InMemoryRegistrar) GetRecord(_ context.Context, fqdn Domain, recordType RecordType) (string, error) {
	if value, present := i.records[strings.ToLower(string(fqdn))+":"+string(recordType)]; present {
		return value, nil
	} else {
		return "", errors.New("Record not present")
	}
}
