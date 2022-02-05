// Package main provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen version v1.9.1 DO NOT EDIT.
package main

import (
	"fmt"
	"net/http"

	"github.com/deepmap/oapi-codegen/pkg/runtime"
	"github.com/go-chi/chi/v5"
)

// Defines values for RecordType.
const (
	RecordTypeA RecordType = "A"

	RecordTypeCNAME RecordType = "CNAME"

	RecordTypeTXT RecordType = "TXT"
)

// RecordType defines model for RecordType.
type RecordType string

// RecordValue defines model for RecordValue.
type RecordValue struct {
	Value *string `json:"value,omitempty"`
}

// Domain defines model for Domain.
type Domain string

// PutDomainJSONBody defines parameters for PutDomain.
type PutDomainJSONBody RecordValue

// PutDomainJSONRequestBody defines body for PutDomain for application/json ContentType.
type PutDomainJSONRequestBody PutDomainJSONBody

// ServerInterface represents all server handlers.
type ServerInterface interface {

	// (GET /domains/{domain}/record/{recordType})
	GetDomain(w http.ResponseWriter, r *http.Request, domain Domain, recordType RecordType)

	// (PUT /domains/{domain}/record/{recordType})
	PutDomain(w http.ResponseWriter, r *http.Request, domain Domain, recordType RecordType)
}

// ServerInterfaceWrapper converts contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler            ServerInterface
	HandlerMiddlewares []MiddlewareFunc
	ErrorHandlerFunc   func(w http.ResponseWriter, r *http.Request, err error)
}

type MiddlewareFunc func(http.HandlerFunc) http.HandlerFunc

// GetDomain operation middleware
func (siw *ServerInterfaceWrapper) GetDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var err error

	// ------------- Path parameter "domain" -------------
	var domain Domain

	err = runtime.BindStyledParameter("simple", false, "domain", chi.URLParam(r, "domain"), &domain)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "domain", Err: err})
		return
	}

	// ------------- Path parameter "recordType" -------------
	var recordType RecordType

	err = runtime.BindStyledParameter("simple", false, "recordType", chi.URLParam(r, "recordType"), &recordType)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "recordType", Err: err})
		return
	}

	var handler = func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.GetDomain(w, r, domain, recordType)
	}

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler(w, r.WithContext(ctx))
}

// PutDomain operation middleware
func (siw *ServerInterfaceWrapper) PutDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var err error

	// ------------- Path parameter "domain" -------------
	var domain Domain

	err = runtime.BindStyledParameter("simple", false, "domain", chi.URLParam(r, "domain"), &domain)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "domain", Err: err})
		return
	}

	// ------------- Path parameter "recordType" -------------
	var recordType RecordType

	err = runtime.BindStyledParameter("simple", false, "recordType", chi.URLParam(r, "recordType"), &recordType)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "recordType", Err: err})
		return
	}

	var handler = func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.PutDomain(w, r, domain, recordType)
	}

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler(w, r.WithContext(ctx))
}

type UnescapedCookieParamError struct {
	ParamName string
	Err       error
}

func (e *UnescapedCookieParamError) Error() string {
	return fmt.Sprintf("error unescaping cookie parameter '%s'", e.ParamName)
}

func (e *UnescapedCookieParamError) Unwrap() error {
	return e.Err
}

type UnmarshalingParamError struct {
	ParamName string
	Err       error
}

func (e *UnmarshalingParamError) Error() string {
	return fmt.Sprintf("Error unmarshaling parameter %s as JSON: %s", e.ParamName, e.Err.Error())
}

func (e *UnmarshalingParamError) Unwrap() error {
	return e.Err
}

type RequiredParamError struct {
	ParamName string
}

func (e *RequiredParamError) Error() string {
	return fmt.Sprintf("Query argument %s is required, but not found", e.ParamName)
}

type RequiredHeaderError struct {
	ParamName string
	Err       error
}

func (e *RequiredHeaderError) Error() string {
	return fmt.Sprintf("Header parameter %s is required, but not found", e.ParamName)
}

func (e *RequiredHeaderError) Unwrap() error {
	return e.Err
}

type InvalidParamFormatError struct {
	ParamName string
	Err       error
}

func (e *InvalidParamFormatError) Error() string {
	return fmt.Sprintf("Invalid format for parameter %s: %s", e.ParamName, e.Err.Error())
}

func (e *InvalidParamFormatError) Unwrap() error {
	return e.Err
}

type TooManyValuesForParamError struct {
	ParamName string
	Count     int
}

func (e *TooManyValuesForParamError) Error() string {
	return fmt.Sprintf("Expected one value for %s, got %d", e.ParamName, e.Count)
}

// Handler creates http.Handler with routing matching OpenAPI spec.
func Handler(si ServerInterface) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{})
}

type ChiServerOptions struct {
	BaseURL          string
	BaseRouter       chi.Router
	Middlewares      []MiddlewareFunc
	ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)
}

// HandlerFromMux creates http.Handler with routing matching OpenAPI spec based on the provided mux.
func HandlerFromMux(si ServerInterface, r chi.Router) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{
		BaseRouter: r,
	})
}

func HandlerFromMuxWithBaseURL(si ServerInterface, r chi.Router, baseURL string) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{
		BaseURL:    baseURL,
		BaseRouter: r,
	})
}

// HandlerWithOptions creates http.Handler with additional options
func HandlerWithOptions(si ServerInterface, options ChiServerOptions) http.Handler {
	r := options.BaseRouter

	if r == nil {
		r = chi.NewRouter()
	}
	if options.ErrorHandlerFunc == nil {
		options.ErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
	wrapper := ServerInterfaceWrapper{
		Handler:            si,
		HandlerMiddlewares: options.Middlewares,
		ErrorHandlerFunc:   options.ErrorHandlerFunc,
	}

	r.Group(func(r chi.Router) {
		r.Get(options.BaseURL+"/domains/{domain}/record/{recordType}", wrapper.GetDomain)
	})
	r.Group(func(r chi.Router) {
		r.Put(options.BaseURL+"/domains/{domain}/record/{recordType}", wrapper.PutDomain)
	})

	return r
}
