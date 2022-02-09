package main

import (
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"strings"
)

type Registrar interface {
	SetRecord(ctx context.Context, fqdn Domain, recordType RecordType, value string)
	GetRecord(ctx context.Context, fqdn Domain, recordType RecordType) (string, error)
}

type InMemoryRegistrar struct {
	records map[string]string
}

func (i InMemoryRegistrar) SetRecord(_ context.Context, fqdn Domain, recordType RecordType, value string) {
	fmt.Printf("Setting <%s> %s -> %s\n", fqdn, recordType, value)
	i.records[strings.ToLower(string(fqdn))+":"+string(recordType)] = value
}

func (i InMemoryRegistrar) GetRecord(_ context.Context, fqdn Domain, recordType RecordType) (string, error) {
	if value, present := i.records[strings.ToLower(string(fqdn))+":"+string(recordType)]; present {
		return value, nil
	} else {
		return "", errors.New("Record not present")
	}
}
