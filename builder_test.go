package main

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestName2(t *testing.T) {
	s := service{}
	by := POST("/:assortment/filters/:id").Decoder(JSONDecoder).By(s.CreateFilters).Encoder(XMLEncoder).ErrorMapping()
	r, err := http.NewRequest(http.MethodPost, "http://localhost:8080/a1/filters/900", strings.NewReader(`["f1", "f2"]`))
	if err != nil {
		t.Fatal(err)
	}

	_, err = by.(*builder).invokeService(r)
	if err != nil {
		t.Error(err)
	}
}

type Filter string
type Key string

// TODO: error mapping: error -> StatusCode

type service struct{}

func (s *service) CreateFilters(assortment string, id uint64, filters []Filter, headers http.Header, queryValues url.Values, cookies []*http.Cookie) (Key, error) {
	fmt.Println(assortment, id, filters, headers, queryValues)
	return "", nil
}

func TestPathValueRange(t *testing.T) {
	for index, toCheck := range []struct {
		uri      string
		from     int
		expected int
	}{
		{uri: "/:bcd", from: 0, expected: 1},
		{uri: "/a/:bcd", from: 0, expected: 3},
	} {
		segment, found := pathValueSegmentOffset(toCheck.uri, toCheck.from)
		if !found {
			t.FailNow()
		}
		if segment != toCheck.expected {
			t.Error("index:", index, "unexpected:", segment, "expects:", toCheck.expected)
		}
	}
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
