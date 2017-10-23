package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

type Interceptor func(w http.ResponseWriter, r *http.Request)

type Decoder func(reader io.Reader) func(v interface{}) error

type Encoder func(writer io.Writer) func(v interface{}) error

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

	headersType    = reflect.TypeOf(http.Header{})
	urlQueryType   = reflect.TypeOf(url.Values{})
	cookiesType    = reflect.TypeOf([]*http.Cookie{})
	errorType      = reflect.TypeOf((*error)(nil)).Elem()
	httpStatusType = reflect.TypeOf(http.StatusOK)

	emptyPathValueRange = [2]int{}
)

const (
	pathParametersGroup = iota
	queryParametersGroup
	headerParametersGroup
	bodyParametersGroup
	cookieParametersGroup

	respBodyParametersGroup
	respErrorParametersGroup
	respStatusCodeParametersGroup
	respHeaderParametersGroup
	respCookieParametersGroup

	pathTemplateStart    = "/:"
	pathTemplateStartLen = len(pathTemplateStart)
	pathTemplateEnd      = "/"
	pathTemplateEndLen   = len(pathTemplateEnd)
)

type Builder interface {
	Before(interceptor Interceptor) Builder
	Decoder(decoder Decoder) Builder
	By(service interface{}) Builder
	Encoder(encoder Encoder) Builder
	After(interceptor Interceptor) Builder
	ErrorMapping() Builder
}

func pathValueSegmentOffsets(requestURI string) []int {
	var offsets []int
	from := 0
	for {
		dirtyOffset := strings.Index(requestURI[from:], pathTemplateStart)
		if dirtyOffset == -1 {
			return offsets
		}
		offset := dirtyOffset + 1
		offsets = append(offsets, offset)

		from += offset
		dirtyOffsetEnd := strings.Index(requestURI[from:], pathTemplateEnd)
		if dirtyOffsetEnd == -1 {
			return offsets
		}
		from += dirtyOffsetEnd
	}
}

func pathValueSegmentOffset(requestURI string, from int) (int, bool) {
	start := strings.Index(requestURI[from:], pathTemplateStart)
	if start == -1 {
		return 0, false
	}
	return start + 1 + from, true
}

func POST(urlPathTemplate string) Builder {
	pathParamsAmount := strings.Count(urlPathTemplate, pathTemplateStart)
	pathValues := func(uri string) []string { return []string{uri} }
	if pathParamsAmount > 0 {
		offsets := pathValueSegmentOffsets(urlPathTemplate)
		pathValues = func(uri string) []string {
			var values []string
			var from int
			for _, offset := range offsets {
				startAt := from + offset
				fmt.Println(uri, from, offset)
				endAt := strings.Index(uri[startAt:], "/")
				if endAt == -1 {
					values = append(values, uri[startAt:])
					return values
				}
				values = append(values, uri[startAt:endAt])
				from = endAt + 1
			}
			return values
		}
	}

	return &builder{
		pathValues:       pathValues,
		pathParamsAmount: pathParamsAmount,
	}
}

type builder struct {
	pathValues       func(uri string) []string
	pathParamsAmount int
	decoder          Decoder
	encoder          Encoder
	err              error
	parametersBy     map[int][]reflect.Type
	serviceValue     reflect.Value

	pathParameters   func(extractedPathValues []string) ([]reflect.Value, error)
	headerParameters func(headers http.Header) ([]reflect.Value, error)
	queryParameters  func(queryValues url.Values) ([]reflect.Value, error)
	cookieParameters func(cookieValues []*http.Cookie) ([]reflect.Value, error)
	bodyParameters   func(bodyReader io.Reader) ([]reflect.Value, error)
}

func (b *builder) Before(interceptor Interceptor) Builder {
	return b
}

func (b *builder) Decoder(decoder Decoder) Builder {
	b.decoder = decoder
	return b
}

func (b *builder) definePathParameters(pathParameters []reflect.Type) {
	if b.err != nil {
		return
	}

	var converters []func(string) (reflect.Value, error)
	for _, pathParameterType := range pathParameters {
		var converter func(string) (reflect.Value, error)
		switch pathParameterType.Kind() {
		case reflect.String:
			converter = func(value string) (reflect.Value, error) { return reflect.ValueOf(value), nil }
		case reflect.Int8:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseInt(value, 10, 8)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(int8(parsed)), nil
			}
		case reflect.Int16:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseInt(value, 10, 16)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(int16(parsed)), nil
			}
		case reflect.Int32:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseInt(value, 10, 32)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(int32(parsed)), nil
			}
		case reflect.Int64:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(int64(parsed)), nil
			}
		case reflect.Int:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseInt(value, 10, 32)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(int(parsed)), nil
			}
		case reflect.Uint8:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseUint(value, 10, 8)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(uint8(parsed)), nil
			}
		case reflect.Uint16:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseUint(value, 10, 16)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(uint16(parsed)), nil
			}
		case reflect.Uint32:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseUint(value, 10, 32)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(uint32(parsed)), nil
			}
		case reflect.Uint64:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(uint64(parsed)), nil
			}
		case reflect.Uint:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseUint(value, 10, 32)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(uint(parsed)), nil
			}
		case reflect.Bool:
			converter = func(value string) (reflect.Value, error) {
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return reflect.Value{}, err
				}
				return reflect.ValueOf(bool(parsed)), nil
			}
		case reflect.Slice, reflect.Array:
			if pathParameterType.Elem().Kind() != reflect.Uint8 {
				b.err = errors.New("supports only slice/array of bytes")
				return
			}
			converter = func(value string) (reflect.Value, error) {
				return reflect.ValueOf([]byte(value)), nil
			}
		default:
			b.err = errors.New("unsupported type for path parameter: " + pathParameterType.String())
			return
		}
		converters = append(converters, converter)
	}

	if len(converters) != 0 {
		b.pathParameters = func(pathValues []string) (values []reflect.Value, err error) {
			amountPathValues := len(pathValues)
			amountConverters := len(converters)
			if amountPathValues != amountConverters {
				return values, fmt.Errorf("unexpected amount of path parameters: %d, expected: %d", amountPathValues, amountConverters)
			}
			for i := 0; i < amountPathValues; i++ {
				var value reflect.Value
				value, err = converters[i](pathValues[i])
				if err != nil {
					return
				}
				values = append(values, value)
			}
			return
		}
	}
}

func (b *builder) By(service interface{}) Builder {
	serviceType := reflect.TypeOf(service)
	if serviceType.Kind() != reflect.Func {
		panic("handler is not a function")
	}

	b.groupParameters(serviceType)
	b.defineProviders()

	b.serviceValue = reflect.ValueOf(service)
	return b
}

// TODO: need to put Values in same order as service function has
func (b *builder) invokeService(r *http.Request) ([]reflect.Value, error) {
	if b.err != nil {
		return nil, b.err
	}

	var invokeValues []reflect.Value

	if b.pathParameters != nil {
		values, err := b.pathParameters(b.pathValues(r.URL.Path))
		if err != nil {
			return nil, err
		}
		invokeValues = append(invokeValues, values...)
	}

	if b.bodyParameters != nil {
		values, err := b.bodyParameters(r.Body)
		if err != nil {
			return nil, err
		}
		fmt.Println("body", values)
		invokeValues = append(invokeValues, values...)
	}

	if b.headerParameters != nil {
		values, err := b.headerParameters(r.Header)
		if err != nil {
			return nil, err
		}
		fmt.Println("header", values)
		invokeValues = append(invokeValues, values...)
	}

	if b.queryParameters != nil {
		values, err := b.queryParameters(r.URL.Query())
		if err != nil {
			return nil, err
		}
		fmt.Println("query", values)
		invokeValues = append(invokeValues, values...)
	}

	if b.cookieParameters != nil {
		values, err := b.cookieParameters(r.Cookies())
		if err != nil {
			return nil, err
		}
		fmt.Println("cookie", values)
		invokeValues = append(invokeValues, values...)
	}

	return b.serviceValue.Call(invokeValues), nil
}

func (b *builder) groupParameters(serviceType reflect.Type) {
	b.groupRequestParameters(serviceType)
	b.groupResponseParameters(serviceType)
}

func (b *builder) groupRequestParameters(serviceType reflect.Type) {
	if b.hasError() {
		return
	}
	b.groupRequestPathParameters(serviceType)
	b.groupRequestOtherParameters(serviceType)
}

func (b *builder) groupRequestPathParameters(serviceType reflect.Type) {
	if b.hasError() {
		return
	}

	if serviceType.NumIn() < b.pathParamsAmount {
		b.err = fmt.Errorf("unexpected amount of path parameters: in URI %d holders, in service function %d receivers", b.pathParamsAmount, serviceType.NumIn())
		return
	}

	b.parametersBy = make(map[int][]reflect.Type)
	for i := 0; i < b.pathParamsAmount; i++ {
		parameterType := serviceType.In(i)
		switch parameterType.Kind() {
		case reflect.String,
			reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		case reflect.Slice, reflect.Array:
			if parameterType.Elem().Kind() != reflect.Uint8 {
				b.err = fmt.Errorf("supports only slice/array of bytes, received: %#v", parameterType.Elem())
				return
			}
		default:
			b.err = fmt.Errorf("unsupported type for path parameter: %#v", parameterType)
			return
		}
		b.parametersBy[pathParametersGroup] = append(b.parametersBy[pathParametersGroup], parameterType)
	}
}

func (b *builder) groupRequestOtherParameters(serviceType reflect.Type) {
	if b.hasError() {
		return
	}

	for i := b.pathParamsAmount; i < serviceType.NumIn(); i++ {
		parameterType := serviceType.In(i)
		switch parameterType {
		case headersType:
			b.parametersBy[headerParametersGroup] = append(b.parametersBy[headerParametersGroup], parameterType)
		case urlQueryType:
			b.parametersBy[queryParametersGroup] = append(b.parametersBy[queryParametersGroup], parameterType)
		case cookiesType:
			b.parametersBy[cookieParametersGroup] = append(b.parametersBy[cookieParametersGroup], parameterType)
		default:
			if len(b.parametersBy[bodyParametersGroup]) > 0 {
				b.err = fmt.Errorf("unable do mapping of body to more than 1 parameter in service function")
				return
			}
			b.parametersBy[bodyParametersGroup] = append(b.parametersBy[bodyParametersGroup], parameterType)
		}
	}
}

func (b *builder) groupResponseParameters(serviceType reflect.Type) {
	if b.hasError() {
		return
	}

	for i := 0; i < serviceType.NumOut(); i++ {
		parameterType := serviceType.Out(i)
		switch {
		case headersType == parameterType:
			b.parametersBy[respHeaderParametersGroup] = append(b.parametersBy[respHeaderParametersGroup], parameterType)
		case cookiesType == parameterType:
			b.parametersBy[respCookieParametersGroup] = append(b.parametersBy[respCookieParametersGroup], parameterType)
		case httpStatusType == parameterType:
			if len(b.parametersBy[respStatusCodeParametersGroup]) > 0 {
				b.err = fmt.Errorf("unable to do mapping of multiple response status codes")
			}
			b.parametersBy[respStatusCodeParametersGroup] = append(b.parametersBy[respStatusCodeParametersGroup], parameterType)
		case parameterType.Implements(errorType):
			if len(b.parametersBy[respErrorParametersGroup]) > 0 {
				b.err = fmt.Errorf("unable to do mapping of multiple error return values")
				return
			}
			b.parametersBy[respErrorParametersGroup] = append(b.parametersBy[respErrorParametersGroup], parameterType)
		default:
			if len(b.parametersBy[respBodyParametersGroup]) > 0 {
				b.err = fmt.Errorf("unable do mapping body to multiple response entities")
			}
			b.parametersBy[respBodyParametersGroup] = append(b.parametersBy[respBodyParametersGroup], parameterType)
		}
	}
}

func (b *builder) inspectRequestParameters(service interface{}) {
	if b.err != nil {
		return
	}
	serviceType := reflect.TypeOf(service)

	var entityPtr reflect.Value

	for i := b.pathParamsAmount; i < serviceType.NumIn(); i++ {
		inputType := serviceType.In(i)
		switch inputType {
		case reflect.TypeOf(url.Values{}):
			fmt.Println("holder for query parameters")
		case reflect.TypeOf(http.Header{}):
			fmt.Println("holder for headers")
		default:
			if entityPtr != (reflect.Value{}) {
				panic("can't handle more than 1 user-defined type for body mapping")
			}
			entityPtr = reflect.New(inputType)
			b.decoder(strings.NewReader(`["hello", "from", "other", "side"]`))(entityPtr)
		}
	}

	reflect.ValueOf(service).Call([]reflect.Value{reflect.ValueOf("a1"), reflect.ValueOf(uint64(100)), entityPtr.Elem(), reflect.Zero(reflect.TypeOf(http.Header{})), reflect.Zero(reflect.TypeOf(url.Values{}))})

}

func (b *builder) defineProviders() {
	if b.hasError() {
		return
	}

	for _, define := range []struct {
		group   int
		definer func(pathParameters []reflect.Type)
	}{
		{pathParametersGroup, b.definePathParameters},
		{headerParametersGroup, b.defineHeaderParameters},
		{queryParametersGroup, b.defineQueryParameters},
		{cookieParametersGroup, b.defineCookieParameters},
		{bodyParametersGroup, b.defineBodyParameters},
	} {
		if b.hasParametersIn(define.group) {
			define.definer(b.parametersBy[define.group])
		}
	}
}

func (b *builder) defineHeaderParameters(headerParameterTypes []reflect.Type) {
	if b.hasError() {
		return
	}

	if len(headerParameterTypes) > 0 {
		b.headerParameters = func(headers http.Header) ([]reflect.Value, error) {
			return []reflect.Value{reflect.ValueOf(headers)}, nil
		}
	}
}

func (b *builder) defineQueryParameters(queryParameterTypes []reflect.Type) {
	if b.hasError() {
		return
	}

	if len(queryParameterTypes) > 0 {
		b.queryParameters = func(queryValues url.Values) ([]reflect.Value, error) {
			return []reflect.Value{reflect.ValueOf(queryValues)}, nil
		}
	}
}

func (b *builder) defineCookieParameters(cookieParameterTypes []reflect.Type) {
	if b.hasError() {
		return
	}

	if len(cookieParameterTypes) > 0 {
		b.cookieParameters = func(cookieValues []*http.Cookie) ([]reflect.Value, error) {
			return []reflect.Value{reflect.ValueOf(cookieValues)}, nil
		}
	}
}

func (b *builder) defineBodyParameters(bodyParameterTypes []reflect.Type) {
	if b.hasError() {
		return
	}

	if len(bodyParameterTypes) > 0 {
		if b.decoder == nil {
			b.err = errors.New("it is not possible to map request body to struct without decoder")
			return
		}
		b.bodyParameters = func(bodyReader io.Reader) ([]reflect.Value, error) {
			entityPtr := reflect.New(bodyParameterTypes[0])
			err := b.decoder(bodyReader)(entityPtr.Interface())
			return []reflect.Value{reflect.Indirect(entityPtr)}, err
		}
	}
}

func (b *builder) hasParametersIn(parametersGroup int) bool {
	return len(b.parametersBy[parametersGroup]) > 0
}

func (b *builder) Encoder(encoder Encoder) Builder {
	b.encoder = encoder
	return b
}

func (b *builder) After(interceptor Interceptor) Builder {
	return b
}

func (b *builder) ErrorMapping() Builder {
	return b
}

func (b *builder) hasError() bool {
	return b.err != nil
}
