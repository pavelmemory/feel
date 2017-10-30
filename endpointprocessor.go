package main

import (
	"net/http"
	"reflect"
)

type EndpointProcessor struct {
	processRequest  func(r *http.Request) ([]reflect.Value, error)
	produceResponse func(executionResult []reflect.Value, executionError error, w http.ResponseWriter, r *http.Request) error
}

func (ep EndpointProcessor) Handle(w http.ResponseWriter, r *http.Request) error {
	results, err := ep.processRequest(r)
	return ep.produceResponse(results, err, w, r)
}
