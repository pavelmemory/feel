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
	"strings"
)

type Interceptor func(w http.ResponseWriter, r *http.Request) bool

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

	pathTemplateStart = "/:"
	pathTemplateEnd   = "/"
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

func POST(urlPathTemplate string) Builder {
	return newBuilder(http.MethodPost, urlPathTemplate)
}

func GET(urlPathTemplate string) Builder {
	return newBuilder(http.MethodGet, urlPathTemplate)
}

func PUT(urlPathTemplate string) Builder {
	return newBuilder(http.MethodPut, urlPathTemplate)
}

func PATCH(urlPathTemplate string) Builder {
	return newBuilder(http.MethodPatch, urlPathTemplate)
}

func DELETE(urlPathTemplate string) Builder {
	return newBuilder(http.MethodDelete, urlPathTemplate)
}

func HEAD(urlPathTemplate string) Builder {
	return newBuilder(http.MethodHead, urlPathTemplate)
}

func CONNECT(urlPathTemplate string) Builder {
	return newBuilder(http.MethodConnect, urlPathTemplate)
}

func OPTIONS(urlPathTemplate string) Builder {
	return newBuilder(http.MethodOptions, urlPathTemplate)
}

func TRACE(urlPathTemplate string) Builder {
	return newBuilder(http.MethodTrace, urlPathTemplate)
}

func pathValuesByOffsets(offsets []int) func(uri string) []string {
	return func(uri string) []string {
		var values []string
		var from int
		for _, offset := range offsets {
			startAt := from + offset
			endAt := strings.Index(uri[startAt:], "/")
			if endAt == -1 {
				values = append(values, uri[startAt:])
				return values
			}
			endAt += startAt
			values = append(values, uri[startAt:endAt])
			from = endAt
		}
		return values
	}
}

func newBuilder(method, urlPathTemplate string) *builder {
	pathParamsAmount := strings.Count(urlPathTemplate, pathTemplateStart)
	var pathValues func(uri string) []string
	if pathParamsAmount > 0 {
		pathValues = pathValuesByOffsets(pathValueSegmentOffsets(urlPathTemplate))
	} else {
		pathValues = func(uri string) []string { return []string{uri} }
	}

	return &builder{
		method:           method,
		pathValues:       pathValues,
		pathParamsAmount: pathParamsAmount,
	}
}

type builder struct {
	method                 string
	pathValues             func(uri string) []string
	pathParamsAmount       int
	decoder                Decoder
	encoder                Encoder
	err                    error
	parametersBy           map[int][]reflect.Type
	serviceValue           reflect.Value
	orderOfOtherParameters []int
	pathParameters         func(extractedPathValues []string) ([]reflect.Value, error)
	headerParameters       func(headers http.Header) (reflect.Value, error)
	queryParameters        func(queryValues url.Values) (reflect.Value, error)
	cookieParameters       func(cookieValues []*http.Cookie) (reflect.Value, error)
	bodyParameters         func(bodyReader io.Reader) (reflect.Value, error)
}

func (b *builder) Before(interceptor Interceptor) Builder {
	return b
}

func (b *builder) Decoder(decoder Decoder) Builder {
	b.decoder = decoder
	return b
}

func (b *builder) definePathParameters() {
	if b.err != nil {
		return
	}

	if !b.hasParametersIn(pathParametersGroup) {
		return
	}

	pathParameters := b.parametersBy[pathParametersGroup]
	var converters []PathParameterConverter
	for _, pathParameterType := range pathParameters {
		var converter PathParameterConverter

		if pathParameterType.Implements(PathParameterConverterType) {
			converter = reflect.New(pathParameterType).Elem().Interface().(PathParameterConverter)
		} else {
			switch pathParameterType.Kind() {
			case reflect.String:
				converter = stringPathParameterConverterSingleton
			case reflect.Int8:
				converter = IntPathParameterConverter{bitSize: 8, valueOf: func(parsed int64) reflect.Value {
					return reflect.ValueOf(int8(parsed))
				}}
			case reflect.Int16:
				converter = IntPathParameterConverter{bitSize: 16, valueOf: func(parsed int64) reflect.Value {
					return reflect.ValueOf(int16(parsed))
				}}
			case reflect.Int32:
				converter = IntPathParameterConverter{bitSize: 32, valueOf: func(parsed int64) reflect.Value {
					return reflect.ValueOf(int32(parsed))
				}}
			case reflect.Int64:
				converter = IntPathParameterConverter{bitSize: 64, valueOf: func(parsed int64) reflect.Value {
					return reflect.ValueOf(parsed)
				}}
			case reflect.Int:
				converter = IntPathParameterConverter{bitSize: 32, valueOf: func(parsed int64) reflect.Value {
					return reflect.ValueOf(int(parsed))
				}}
			case reflect.Uint8:
				converter = UintPathParameterConverter{bitSize: 8, valueOf: func(parsed uint64) reflect.Value {
					return reflect.ValueOf(uint8(parsed))
				}}
			case reflect.Uint16:
				converter = UintPathParameterConverter{bitSize: 16, valueOf: func(parsed uint64) reflect.Value {
					return reflect.ValueOf(uint16(parsed))
				}}
			case reflect.Uint32:
				converter = UintPathParameterConverter{bitSize: 32, valueOf: func(parsed uint64) reflect.Value {
					return reflect.ValueOf(uint32(parsed))
				}}
			case reflect.Uint64:
				converter = UintPathParameterConverter{bitSize: 64, valueOf: func(parsed uint64) reflect.Value {
					return reflect.ValueOf(parsed)
				}}
			case reflect.Uint:
				converter = UintPathParameterConverter{bitSize: 32, valueOf: func(parsed uint64) reflect.Value {
					return reflect.ValueOf(uint(parsed))
				}}
			case reflect.Bool:
				converter = boolPathParameterConverterSingleton
			case reflect.Slice:
				if pathParameterType.Elem().Kind() != reflect.Uint8 {
					b.err = UnsupportedTypeError(errors.New("supports only slice/array of bytes"))
					return
				}
				converter = sliceBytePathParameterConverterSingleton
			case reflect.Array:
				if pathParameterType.Elem().Kind() != reflect.Uint8 {
					b.err = UnsupportedTypeError(errors.New("supports only array of bytes"))
					return
				}
				converter = ArrayBytePathParameterConverter{length: pathParameterType.Len(), elementType: pathParameterType.Elem()}
			default:
				b.err = UnsupportedTypeError(errors.New("for path parameter: " + pathParameterType.String()))
				return
			}
		}
		converters = append(converters, converter)
	}

	if len(converters) != 0 {
		b.pathParameters = func(pathValues []string) (values []reflect.Value, err error) {
			amountPathValues := len(pathValues)
			amountConverters := len(converters)
			if amountPathValues != amountConverters {
				return values, InvalidMappingError(fmt.Errorf("unexpected amount of path parameters: %d, expected: %d", amountPathValues, amountConverters))
			}
			for i := 0; i < amountPathValues; i++ {
				var value reflect.Value
				value, err = converters[i].Convert(pathValues[i])
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
		b.err = InvalidMappingError(errors.New("handler is not a function/method"))
		return b
	}

	b.groupParameters(serviceType)
	b.defineProviders()

	b.serviceValue = reflect.ValueOf(service)
	return b
}

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

	for _, group := range b.orderOfOtherParameters {
		var value reflect.Value
		var err error
		switch group {
		case headerParametersGroup:
			value, err = b.headerParameters(r.Header)
		case queryParametersGroup:
			value, err = b.queryParameters(r.URL.Query())
		case cookieParametersGroup:
			value, err = b.cookieParameters(r.Cookies())
		case bodyParametersGroup:
			value, err = b.bodyParameters(r.Body)
		default:
			b.err = fmt.Errorf("undefined group in the order list: %d", group)
		}
		if err != nil {
			return nil, err
		}
		invokeValues = append(invokeValues, value)
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

	addToGroup := func(parameterType reflect.Type, errorMsg string, group int) bool {
		if len(b.parametersBy[group]) > 0 {
			b.err = InvalidMappingError(errors.New(errorMsg))
			return false
		}
		b.parametersBy[group] = append(b.parametersBy[group], parameterType)
		b.orderOfOtherParameters = append(b.orderOfOtherParameters, group)
		return true
	}

	noError := true
	for i := b.pathParamsAmount; noError && i < serviceType.NumIn(); i++ {
		parameterType := serviceType.In(i)
		switch parameterType {
		case headersType:
			noError = addToGroup(parameterType, "unable do mapping of headers to more than 1 parameter in service function", headerParametersGroup)
		case urlQueryType:
			noError = addToGroup(parameterType, "unable do mapping of URL query values to more than 1 parameter in service function", queryParametersGroup)
		case cookiesType:
			noError = addToGroup(parameterType, "unable do mapping of cookies to more than 1 parameter in service function", cookieParametersGroup)
		default:
			noError = addToGroup(parameterType, "unable do mapping of body to more than 1 parameter in service function", bodyParametersGroup)
		}
	}
}

// TODO: do convertion of response params to HTTP response
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
				b.err = InvalidMappingError(errors.New("unable to map multiple response status codes"))
				return
			}
			b.parametersBy[respStatusCodeParametersGroup] = append(b.parametersBy[respStatusCodeParametersGroup], parameterType)
		case parameterType.Implements(errorType):
			if len(b.parametersBy[respErrorParametersGroup]) > 0 {
				b.err = InvalidMappingError(errors.New("unable to map multiple error return values"))
				return
			}
			b.parametersBy[respErrorParametersGroup] = append(b.parametersBy[respErrorParametersGroup], parameterType)
		default:
			if len(b.parametersBy[respBodyParametersGroup]) > 0 {
				b.err = InvalidMappingError(errors.New("unable to map body to multiple response entities"))
			}
			b.parametersBy[respBodyParametersGroup] = append(b.parametersBy[respBodyParametersGroup], parameterType)
		}
	}
}

func (b *builder) defineProviders() {
	if b.hasError() {
		return
	}

	b.definePathParameters()
	b.defineHeaderParameters()
	b.defineQueryParameters()
	b.defineCookieParameters()
	b.defineBodyParameters()
}

func (b *builder) defineHeaderParameters() {
	if b.hasError() {
		return
	}

	if !b.hasParametersIn(headerParametersGroup) {
		return
	}

	headerParameterTypes := b.parametersBy[headerParametersGroup]
	if len(headerParameterTypes) > 0 {
		b.headerParameters = func(headers http.Header) (reflect.Value, error) {
			return reflect.ValueOf(headers), nil
		}
	}
}

func (b *builder) defineQueryParameters() {
	if b.hasError() {
		return
	}

	if !b.hasParametersIn(queryParametersGroup) {
		return
	}

	queryParameterTypes := b.parametersBy[queryParametersGroup]
	if len(queryParameterTypes) > 0 {
		b.queryParameters = func(queryValues url.Values) (reflect.Value, error) {
			return reflect.ValueOf(queryValues), nil
		}
	}
}

func (b *builder) defineCookieParameters() {
	if b.hasError() {
		return
	}

	if !b.hasParametersIn(cookieParametersGroup) {
		return
	}

	cookieParameterTypes := b.parametersBy[cookieParametersGroup]
	if len(cookieParameterTypes) > 0 {
		b.cookieParameters = func(cookieValues []*http.Cookie) (reflect.Value, error) {
			return reflect.ValueOf(cookieValues), nil
		}
	}
}

func (b *builder) defineBodyParameters() {
	if b.hasError() {
		return
	}

	if !b.hasParametersIn(bodyParametersGroup) {
		return
	}

	bodyParameterTypes := b.parametersBy[bodyParametersGroup]
	if len(bodyParameterTypes) > 0 {
		if b.decoder == nil {
			b.err = errors.New("it is not possible to map request body to struct without decoder")
			return
		}
		b.bodyParameters = func(bodyReader io.Reader) (reflect.Value, error) {
			entityPtr := reflect.New(bodyParameterTypes[0])
			if bodyReader == nil {
				return entityPtr.Elem(), nil
			}
			err := b.decoder(bodyReader)(entityPtr.Interface())
			return reflect.Indirect(entityPtr), err
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

// TODO: body mapping is not implemented
// TODO: error mapping: error -> StatusCode
// TODO: Header parameters into user-defined types - ???
// maybe there will be some policy in naming those user-defined types
// TODO: make normal error reporting with error codes that signals generic cause and context specific info (maybe stack-trace)
// TODO: make normal tests - visually check of prints is not good enough

type GeneralErrorCause error

var (
	UnsupportedType = errors.New("unsupported type")
	InvalidMapping  = errors.New("invalid mapping")
)

func UnsupportedTypeError(contextCause error) error {
	return Error{GeneralCause: UnsupportedType, ContextCause: contextCause}
}

func InvalidMappingError(contextCause error) error {
	return Error{GeneralCause: InvalidMapping, ContextCause: contextCause}
}

type Error struct {
	GeneralCause GeneralErrorCause
	ContextCause error
}

func (e Error) Error() string {
	switch {
	case e.GeneralCause != nil && e.ContextCause != nil:
		return e.GeneralCause.Error() + ":" + e.ContextCause.Error()
	case e.GeneralCause != nil:
		return e.GeneralCause.Error()
	case e.ContextCause != nil:
		return e.ContextCause.Error()
	}
	return ""
}
