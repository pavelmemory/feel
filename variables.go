package main

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"reflect"
)

var (
	JSONDecoder = func(reader io.Reader) func(v interface{}) error {
		return json.NewDecoder(reader).Decode
	}

	JSONEncoder = func(writer io.Writer) func(v interface{}) error {
		return json.NewEncoder(writer).Encode
	}

	XMLDecoder = func(reader io.Reader) func(v interface{}) error {
		return xml.NewDecoder(reader).Decode
	}

	XMLEncoder = func(writer io.Writer) func(v interface{}) error {
		return xml.NewEncoder(writer).Encode
	}

	DefaultErrorMapper ErrorMapper = func(err error, w http.ResponseWriter, r *http.Request) error {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	Application = struct {
		JSON ContentType
		XML  ContentType
		ZIP  ContentType
		GZIP ContentType
		PDF  ContentType
	}{
		JSON: func() string {
			return "application/json; charset=utf-8"
		},
		XML: func() string {
			return "application/xml; charset=utf-8"
		},
		ZIP: func() string {
			return "application/zip"
		},
		GZIP: func() string {
			return "application/gzip"
		},
		PDF: func() string {
			return "application/pdf; charset=utf-8"
		},
	}

	Multipart = struct {
		FormData ContentType
	}{
		FormData: func() string {
			return "multipart/form-data; charset=utf-8"
		},
	}

	Text = struct {
		CMD   ContentType
		CSS   ContentType
		CSV   ContentType
		HTML  ContentType
		Plain ContentType
		XML   ContentType
	}{
		CMD: func() string {
			return "text/cmd; charset=utf-8"
		},
		CSS: func() string {
			return "text/css; charset=utf-8"
		},
		CSV: func() string {
			return "text/csv; charset=utf-8"
		},
		HTML: func() string {
			return "text/html; charset=utf-8"
		},
		Plain: func() string {
			return "text/plain; charset=utf-8"
		},
		XML: func() string {
			return "text/xml; charset=utf-8"
		},
	}

	headersType    = reflect.TypeOf(http.Header{})
	urlQueryType   = reflect.TypeOf(url.Values{})
	cookiesType    = reflect.TypeOf([]*http.Cookie{})
	errorType      = reflect.TypeOf((*error)(nil)).Elem()
	httpStatusType = reflect.TypeOf(http.StatusOK)
)
