package main

import (
	"golang.org/x/net/context"
)

type Registrar interface {
	SetRecord(ctx context.Context, fqdn Domain, recordType RecordType, value string) error
	GetRecord(ctx context.Context, fqdn Domain, recordType RecordType) (string, error)
	DeleteRecord(ctx context.Context, fqdn Domain, recordType RecordType, currentValue string) error
}
