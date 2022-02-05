package main

import "errors"

type Registrar interface {
	SetRecord(fqdn string, recordType string, value string)
	GetRecord(fqdn string, recordType string) (string, error)
}

type InMemoryRegistrar struct {
	records map[string]string
}

func (i InMemoryRegistrar) SetRecord(fqdn string, recordType string, value string) {
	i.records[fqdn+":"+recordType] = value
}

func (i InMemoryRegistrar) GetRecord(fqdn string, recordType string) (string, error) {
	if value, present := i.records[fqdn+":"+recordType]; present {
		return value, nil
	} else {
		return "", errors.New("Record not present")
	}

}

