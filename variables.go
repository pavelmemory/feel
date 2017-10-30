package main

import (
	"io"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"reflect"
	"net/url"
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
		w.WriteHeader(http.StatusInternalServerError)
		_, wErr := w.Write([]byte(err.Error()))
		return wErr
	}

	headersType    = reflect.TypeOf(http.Header{})
	urlQueryType   = reflect.TypeOf(url.Values{})
	cookiesType    = reflect.TypeOf([]*http.Cookie{})
	errorType      = reflect.TypeOf((*error)(nil)).Elem()
	httpStatusType = reflect.TypeOf(http.StatusOK)
)
