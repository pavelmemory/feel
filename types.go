package main

import (
	"net/http"
	"io"
)

type Interceptor func(w http.ResponseWriter, r *http.Request) bool

type Decoder func(reader io.Reader) func(v interface{}) error

type Encoder func(writer io.Writer) func(v interface{}) error

type ErrorMapper func(err error, w http.ResponseWriter, r *http.Request) error
