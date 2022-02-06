package main

import (
	"errors"
	"fmt"
)

type Registrar interface {
	SetRecord(fqdn Domain, recordType RecordType, value string)
	GetRecord(fqdn Domain, recordType RecordType) (string, error)
}

type InMemoryRegistrar struct {
	records map[string]string
}

func (i InMemoryRegistrar) SetRecord(fqdn Domain, recordType RecordType, value string) {
	fmt.Printf("Setting <%s> %s -> %s\n", fqdn, recordType, value)
	i.records[string(fqdn)+":"+string(recordType)] = value
}

func (i InMemoryRegistrar) GetRecord(fqdn Domain, recordType RecordType) (string, error) {
	if value, present := i.records[string(fqdn)+":"+string(recordType)]; present {
		return value, nil
	} else {
		return "", errors.New("Record not present")
	}
}
