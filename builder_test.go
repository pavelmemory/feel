package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type Filter string

type Key struct {
	Value string `xml:"value"`
	Part  int16  `xml:"position"`
}

type service struct {
	t             *testing.T
	createFilters func(assortment string, id uint64, queryValues url.Values, headers http.Header, filters []Filter, cookies []*http.Cookie) (int, Key, error)
}

func (s *service) CreateFilters(assortment string, id uint64, queryValues url.Values, headers http.Header, filters []Filter, cookies []*http.Cookie) (int, Key, error) {
	return s.createFilters(assortment, id, queryValues, headers, filters, cookies)
}

func TestAll(t *testing.T) {
	s := service{createFilters: func(assortment string, id uint64, queryValues url.Values, headers http.Header, filters []Filter, cookies []*http.Cookie) (int, Key, error) {
		if assortment != "a1" {
			t.Errorf("received: %#v", assortment)
		}
		if id != 900 {
			t.Errorf("received: %#v", id)
		}
		if len(queryValues) != 2 {
			t.Errorf("received: %#v", queryValues)
		}
		if queryValues.Get("qv1") != "100" {
			t.Errorf("received: %#v", queryValues.Get("qv1"))
		}
		if queryValues.Get("qv2") != "oops?" {
			t.Errorf("received: %#v", queryValues.Get("qv2"))
		}
		if len(headers) != 2 {
			t.Errorf("received: %#v", headers)
		}
		h1HeaderKey := textproto.CanonicalMIMEHeaderKey("h1")
		if headers[h1HeaderKey][0] != "v1" {
			t.Errorf("received: %#v", headers[h1HeaderKey][0])
		}
		if headers[h1HeaderKey][1] != "v2" {
			t.Errorf("received: %#v", headers[h1HeaderKey][1])
		}
		if len(filters) != 2 {
			t.Errorf("received: %#v", filters)
		}
		if filters[0] != "f1" {
			t.Errorf("received: %#v", filters[0])
		}
		if filters[1] != "f2" {
			t.Errorf("received: %#v", filters[1])
		}
		if len(cookies) != 2 {
			t.Errorf("received: %#v", cookies)
		}
		if cookies[0].Name != "c1" || cookies[0].Value != "cv1" {
			t.Errorf("received: %#v", cookies[0])
		}
		if cookies[1].Name != "c2" || cookies[1].Value != "cv2" {
			t.Errorf("received: %#v", cookies[1])
		}
		return http.StatusFound, Key{Value: "R&R", Part: 3}, nil
	}}

	by := POST("/:assortment/filters/:id").
		Decoder(JSONDecoder).
		Handler(s.CreateFilters).
		ResponseContentType(Application.XML).
		Encoder(XMLEncoder).
		ErrorMapping(DefaultErrorMapper)

	r := newPOST(t, "http://localhost:8080/a1/filters/900?qv1=100&qv2=oops%3F", strings.NewReader(`["f1", "f2"]`))
	r.Header.Set("h1", "v1")
	r.Header.Add("h1", "v2")
	r.AddCookie(&http.Cookie{Name: "c1", Value: "cv1"})
	r.AddCookie(&http.Cookie{Name: "c2", Value: "cv2"})
	w := httptest.NewRecorder()

	b := by.(builder).Build()
	err := b.Handle(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusFound {
		t.Error("unexpected HTTP response status", w.Code)
	}
	contentType := w.Header().Get("Content-Type")
	mediaType, mediaTypeParams, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "application/xml" {
		t.Error("unexpected HTTP Content-Type", contentType)
	}
	if mediaTypeParams["charset"] != "utf-8" {
		t.Error("unexpected HTTP Content-Type charset", mediaTypeParams["charset"])
	}

	var result Key
	err = xml.NewDecoder(w.Body).Decode(&result)
	if err != nil {
		t.Error(err)
	}
	if result.Value != "R&R" || result.Part != 3 {
		t.Error("unexpected entity", result)
	}
}

func (s *service) ArrayAsPathParameterHolder(assortment []byte) {
	if string(assortment) != "a1" {
		s.t.Errorf("receive: %#v", assortment)
	}
}

func TestArrayAsPathParameterHolder(t *testing.T) {
	s := service{t: t}
	by := GET("/:assortment").Handler(s.ArrayAsPathParameterHolder)
	r := newGET(t, "http://localhost:8080/a1")
	w := &httptest.ResponseRecorder{}

	b := by.(builder).Build()
	err := b.Handle(w, r)
	if err != nil {
		t.Error(err)
	}
}

func (s *service) MultiplePathParameterHolders(id uint16, assortment string) {
	if id != 666 {
		s.t.Errorf("receive: %#v", id)
	}
	if assortment != "POOW" {
		s.t.Errorf("receive: %#v", assortment)
	}
}

func TestMultiplePathParameterHolders(t *testing.T) {
	s := service{t: t}
	by := GET("/some/part/:id/:assortment/here").Handler(s.MultiplePathParameterHolders)
	r := newGET(t, "http://localhost:8080/some/part/666/POOW/here")
	w := &httptest.ResponseRecorder{}

	b := by.(builder).Build()
	err := b.Handle(w, r)
	if err != nil {
		t.Error(err)
	}
}

type UserDefinedPathType string

var _ PathParameterConverter = UserDefinedPathType("")

func (UserDefinedPathType) Convert(pathPart string) (reflect.Value, error) {
	udp := UserDefinedPathType(pathPart)
	return reflect.ValueOf(udp), nil
}

func (ud UserDefinedPathType) String() string {
	return "UserDefinedPathType: " + string(ud)
}

func (s *service) UserDefinedTypeAsPathParameterHolder(assortment UserDefinedPathType) {
	if assortment.String() != "UserDefinedPathType: a1" {
		s.t.Errorf("receive: %#v", assortment)
	}
}

func TestUserDefinedTypeAsPathParameterHolder(t *testing.T) {
	s := service{t: t}
	by := GET("/:assortment").Handler(s.UserDefinedTypeAsPathParameterHolder)
	r := newGET(t, "http://localhost:8080/a1")
	w := &httptest.ResponseRecorder{}

	b := by.(builder).Build()
	err := b.Handle(w, r)
	if err != nil {
		t.Error(err)
	}
}

func TestErrorMapper(t *testing.T) {
	expected := errors.New("handled")
	s := service{createFilters: func(assortment string, id uint64, queryValues url.Values, headers http.Header, filters []Filter, cookies []*http.Cookie) (int, Key, error) {
		return 0, Key{}, expected
	}}
	by := POST("/:/:").Encoder(JSONEncoder).Decoder(JSONDecoder).Handler(s.CreateFilters).ErrorMapping(func(err error, w http.ResponseWriter, r *http.Request) error {
		if err != expected {
			t.Error(err)
		}
		w.Header().Set("k1", "v1")
		w.WriteHeader(http.StatusBadRequest)
		return nil
	})
	r := newPOST(t, "http://localhost:8080/a/1", strings.NewReader("[]"))
	w := &httptest.ResponseRecorder{}

	err := by.Build().Handle(w, r)
	if err != nil {
		t.Error(err)
	}
	if w.Code != http.StatusBadRequest {
		t.Error("unexpected response code", w.Code)
	}
	if w.Header().Get("k1") != "v1" {
		t.Error("unexpected headers", w.Header())
	}
}

func TestStatusCode(t *testing.T) {
	expected := http.StatusAlreadyReported
	by := GET("/").Handler(func() int {
		return expected
	})

	r := newGET(t, "http://localhost")
	w := &httptest.ResponseRecorder{}

	err := by.Build().Handle(w, r)
	if err != nil {
		t.Error(err)
	}
	if w.Code != expected {
		t.Error("unexpected response code", w.Code)
	}
}

func TestDefaultErrorMapper(t *testing.T) {
	by := GET("/").Handler(func() error {
		return errors.New("some error")
	})

	r := newGET(t, "http://localhost")
	w := &httptest.ResponseRecorder{Body: &bytes.Buffer{}}

	err := by.Build().Handle(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusInternalServerError {
		t.Error("unexpected response code", w.Code)
	}
	data, err := ioutil.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "some error" {
		t.Error("unexpected response body:", string(data))
	}
}

func TestDump(t *testing.T) {
	by := GET("/").Handler(func() {})
	r := newGET(t, "http://localhost")
	w := &httptest.ResponseRecorder{}

	err := by.Build().Handle(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK {
		t.Error("unexpected response code", w.Code)
	}
	if len(w.Header()) > 0 {
		t.Error("enexpected:", w.Header())
	}
}

func newPOST(t *testing.T, urlString string, body io.Reader) *http.Request {
	return newRequest(t, http.MethodPost, urlString, body)
}

func newGET(t *testing.T, urlString string) *http.Request {
	return newRequest(t, http.MethodGet, urlString, nil)
}

func newRequest(t *testing.T, httpMethod, urlString string, body io.Reader) *http.Request {
	r, err := http.NewRequest(httpMethod, urlString, body)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPathValueSegmentOffsets(t *testing.T) {
	for index, toCheck := range []struct {
		uri      string
		expected []int
	}{
		{uri: "/abc/def", expected: nil},
		{uri: "/:bcd", expected: []int{1}},
		{uri: "/a/:bcd", expected: []int{3}},
		{uri: "/a/:bcd/ef/:", expected: []int{3, 4}},
		{uri: "/a/:bcd/:/ef", expected: []int{3, 1}},
	} {
		offsets := pathValueSegmentOffsets(toCheck.uri)
		if !reflect.DeepEqual(offsets, toCheck.expected) {
			t.Error("index:", index, "unexpected:", offsets, "expects:", toCheck.expected)
		}
	}
}
