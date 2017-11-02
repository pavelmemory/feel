package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

const (
	pathParametersGroup = iota
	queryParametersGroup
	headerParametersGroup
	bodyParametersGroup
	cookieParametersGroup

	responseBodyParametersGroup
	responseErrorParametersGroup
	responseStatusCodeParametersGroup
	responseHeaderParametersGroup
	responseContentTypeParametersGroup
	responseCookieParametersGroup

	pathTemplateStart = "/:"
	pathTemplateEnd   = "/"
)

type Builder interface {
	Before(interceptor Interceptor) Builder
	Decoder(decoder Decoder) Builder
	Handler(service interface{}) Builder
	Encoder(encoder Encoder) Builder
	ResponseContentType(setter ContentType) Builder
	After(interceptor Interceptor) Builder
	ErrorMapping(errorMapper ErrorMapper) Builder
	Build() EndpointProcessor
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

func newBuilder(method, urlPathTemplate string) builder {
	pathParamsAmount := strings.Count(urlPathTemplate, pathTemplateStart)
	var pathValues func(uri string) []string
	if pathParamsAmount > 0 {
		pathValues = pathValuesByOffsets(pathValueSegmentOffsets(urlPathTemplate))
	} else {
		pathValues = func(uri string) []string { return []string{uri} }
	}

	return builder{
		method:           method,
		pathValues:       pathValues,
		pathParamsAmount: pathParamsAmount,
		errors:           []error{},
	}
}

type builder struct {
	method                 string
	pathValues             func(uri string) []string
	pathParamsAmount       int
	decoder                Decoder
	contentTypeProvider    ContentType
	encoder                Encoder
	errors                 []error
	parametersBy           map[int][]reflect.Type
	serviceValue           reflect.Value
	orderOfOtherParameters []int
	pathParameters         func(extractedPathValues []string) ([]reflect.Value, error)
	headerParameters       func(headers http.Header) (reflect.Value, error)
	queryParameters        func(queryValues url.Values) (reflect.Value, error)
	cookieParameters       func(cookieValues []*http.Cookie) (reflect.Value, error)
	bodyParameters         func(bodyReader io.Reader) (reflect.Value, error)

	errorMapper                  ErrorMapper
	orderOfResponseParameters    []int
	responseHeaderParameters     func(value reflect.Value) http.Header
	responseStatusCodeParameters func(value reflect.Value) int
	responseCookieParameters     func(value reflect.Value) []*http.Cookie
	responseErrorParameters      func(err error, w http.ResponseWriter, r *http.Request) error
}

func (cloned builder) clone() builder {
	if len(cloned.parametersBy) > 0 {
		parametersBy := cloned.parametersBy
		cloned.parametersBy = make(map[int][]reflect.Type)
		for key, value := range parametersBy {
			valueCloned := make([]reflect.Type, len(value))
			copy(valueCloned, value)
			cloned.parametersBy[key] = valueCloned
		}
	}

	if len(cloned.orderOfOtherParameters) > 0 {
		orderOfOtherParameters := cloned.orderOfOtherParameters
		cloned.orderOfOtherParameters = make([]int, len(orderOfOtherParameters))
		copy(cloned.orderOfOtherParameters, orderOfOtherParameters)
	}

	if len(cloned.orderOfResponseParameters) > 0 {
		orderOfResponseParameters := cloned.orderOfResponseParameters
		cloned.orderOfResponseParameters = make([]int, len(orderOfResponseParameters))
		copy(cloned.orderOfResponseParameters, orderOfResponseParameters)
	}

	if len(cloned.errors) > 0 {
		errs := cloned.errors
		cloned.errors = make([]error, len(errs))
		copy(cloned.errors, errs)
	}
	return cloned
}

// TODO: how to put before interceptors?
// Would it be a traditional chain call?
// Do we want interceptors to be any kind of functions with same mapping rules that main service function apply to?
// Or just implement a specific interface?
func (b builder) Before(interceptor Interceptor) Builder {
	cloned := b.clone()
	//cloned.before = interceptor
	return cloned
}

func (b builder) Decoder(decoder Decoder) Builder {
	cloned := b.clone()
	cloned.decoder = decoder
	return cloned
}

func (b builder) ResponseContentType(setter ContentType) Builder {
	cloned := b.clone()
	cloned.contentTypeProvider = setter
	return cloned
}

func (b *builder) definePathParameters() {
	pathParameters, exist := b.hasParametersIn(pathParametersGroup)
	if !exist {
		return
	}

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
					b.errors = append(b.errors, UnsupportedTypeError(errors.New("supports only slice/array of bytes")))
					return
				}
				converter = sliceBytePathParameterConverterSingleton
			case reflect.Array:
				returnParameterTypeElem := pathParameterType.Elem()
				if returnParameterTypeElem.Kind() != reflect.Uint8 {
					b.errors = append(b.errors, UnsupportedTypeError(errors.New("supports only array of bytes")))
					return
				}
				converter = ArrayBytePathParameterConverter{length: pathParameterType.Len(), elementType: returnParameterTypeElem}
			default:
				b.errors = append(b.errors, UnsupportedTypeError(errors.New("for path parameter: "+pathParameterType.String())))
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

func (b builder) Handler(service interface{}) Builder {
	serviceType := reflect.TypeOf(service)
	if serviceType.Kind() != reflect.Func {
		b.errors = append(b.errors, InvalidMappingError(errors.New("handler is not a function/method")))
		return b
	}
	cloned := b.clone()
	cloned.serviceValue = reflect.ValueOf(service)
	return cloned
}

func (b *builder) groupParameters(serviceType reflect.Type) {
	b.groupRequestParameters(serviceType)
	b.groupResponseParameters(serviceType)
}

func (b *builder) groupRequestParameters(serviceType reflect.Type) {
	b.groupRequestPathParameters(serviceType)
	b.groupRequestOtherParameters(serviceType)
}

func (b *builder) groupRequestPathParameters(serviceType reflect.Type) {
	if serviceType.NumIn() < b.pathParamsAmount {
		b.errors = append(b.errors, InvalidMappingError(fmt.Errorf("unexpected amount of path parameters: in URI %d holders, in service function %d receivers", b.pathParamsAmount, serviceType.NumIn())))
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
			returnParameterTypeElem := parameterType.Elem()
			if returnParameterTypeElem.Kind() != reflect.Uint8 {
				b.errors = append(b.errors, UnsupportedTypeError(fmt.Errorf("supports only slice/array of bytes, received: %#v", returnParameterTypeElem)))
				return
			}
		default:
			b.errors = append(b.errors, UnsupportedTypeError(fmt.Errorf("unsupported type for path parameter: %#v", parameterType)))
			return
		}
		b.parametersBy[pathParametersGroup] = append(b.parametersBy[pathParametersGroup], parameterType)
	}
}

func (b *builder) groupRequestOtherParameters(serviceType reflect.Type) {
	addToGroup := func(parameterType reflect.Type, errorMsg string, group int) bool {
		if len(b.parametersBy[group]) > 0 {
			b.errors = append(b.errors, InvalidMappingError(errors.New(errorMsg)))
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

func (b *builder) groupResponseParameters(serviceType reflect.Type) {
	for i := 0; i < serviceType.NumOut(); i++ {
		parameterType := serviceType.Out(i)
		switch {
		case headersType == parameterType:
			group := responseHeaderParametersGroup
			b.parametersBy[group] = append(b.parametersBy[group], parameterType)
			b.orderOfResponseParameters = append(b.orderOfResponseParameters, group)
		case cookiesType == parameterType:
			group := responseCookieParametersGroup
			b.parametersBy[group] = append(b.parametersBy[group], parameterType)
			b.orderOfResponseParameters = append(b.orderOfResponseParameters, group)
		case httpStatusType == parameterType:
			group := responseStatusCodeParametersGroup
			responseStatusCodeParametersGroupTypes := b.parametersBy[group]
			if len(responseStatusCodeParametersGroupTypes) > 0 {
				b.errors = append(b.errors, InvalidMappingError(errors.New("unable to map multiple response status codes")))
				return
			}
			b.parametersBy[group] = append(responseStatusCodeParametersGroupTypes, parameterType)
			b.orderOfResponseParameters = append(b.orderOfResponseParameters, group)
		case parameterType.Implements(errorType):
			group := responseErrorParametersGroup
			responseErrorParametersGroupTypes := b.parametersBy[group]
			if len(responseErrorParametersGroupTypes) > 0 {
				b.errors = append(b.errors, InvalidMappingError(errors.New("unable to map multiple error return values")))
				return
			}
			b.parametersBy[group] = append(responseErrorParametersGroupTypes, parameterType)
			b.orderOfResponseParameters = append(b.orderOfResponseParameters, group)
		default:
			group := responseBodyParametersGroup
			responseBodyParametersGroupTypes := b.parametersBy[group]
			if len(responseBodyParametersGroupTypes) > 0 {
				b.errors = append(b.errors, InvalidMappingError(errors.New("unable to map body to multiple response entities")))
			}
			b.parametersBy[group] = append(responseBodyParametersGroupTypes, parameterType)
			b.orderOfResponseParameters = append(b.orderOfResponseParameters, group)
		}
	}
}

func (b *builder) defineProviders() {
	b.definePathParameters()
	b.defineHeaderParameters()
	b.defineQueryParameters()
	b.defineCookieParameters()
	b.defineBodyParameters()

	b.defineResponseHeaderParameters()
	b.defineResponseStatusCodeParameters()
	b.defineResponseCookieParameters()
	b.defineResponseErrorParameters()
}

func (b *builder) defineHeaderParameters() {
	headerParameterTypes, exist := b.hasParametersIn(headerParametersGroup)
	if !exist {
		return
	}

	if len(headerParameterTypes) > 0 {
		b.headerParameters = func(headers http.Header) (reflect.Value, error) {
			return reflect.ValueOf(headers), nil
		}
	}
}

func (b *builder) defineQueryParameters() {
	queryParameterTypes, exist := b.hasParametersIn(queryParametersGroup)
	if !exist {
		return
	}

	if len(queryParameterTypes) > 0 {
		b.queryParameters = func(queryValues url.Values) (reflect.Value, error) {
			return reflect.ValueOf(queryValues), nil
		}
	}
}

func (b *builder) defineCookieParameters() {
	cookieParameterTypes, exist := b.hasParametersIn(cookieParametersGroup)
	if !exist {
		return
	}

	if len(cookieParameterTypes) > 0 {
		b.cookieParameters = func(cookieValues []*http.Cookie) (reflect.Value, error) {
			return reflect.ValueOf(cookieValues), nil
		}
	}
}

func (b *builder) defineBodyParameters() {
	bodyParameterTypes, exist := b.hasParametersIn(bodyParametersGroup)
	if !exist {
		return
	}

	if len(bodyParameterTypes) != 1 {
		b.errors = append(b.errors, InvalidMappingError(errors.New("doesn't support multiple return body mapped values")))
		return
	}
	if b.decoder == nil {
		b.errors = append(b.errors, InvalidMappingError(errors.New("mapping of request body to struct without decoder is impossible")))
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
	return
}

func (b *builder) defineResponseHeaderParameters() {
	headerParameterTypes, exist := b.hasParametersIn(responseHeaderParametersGroup)
	if !exist {
		return
	}

	if len(headerParameterTypes) != 1 {
		b.errors = append(b.errors, InvalidMappingError(errors.New("supports only single response headers service function return value")))
		return
	}

	b.responseHeaderParameters = func(value reflect.Value) http.Header {
		if value.IsNil() {
			return nil
		}
		return value.Interface().(http.Header)
	}
}

func (b *builder) defineResponseStatusCodeParameters() {
	responseStatusCodeTypes, exist := b.hasParametersIn(responseStatusCodeParametersGroup)
	if !exist {
		return
	}

	if len(responseStatusCodeTypes) != 1 {
		b.errors = append(b.errors, InvalidMappingError(errors.New("supports only single response status code service function return value")))
		return
	}

	b.responseStatusCodeParameters = func(statusCodeValue reflect.Value) int {
		return int(statusCodeValue.Int())
	}
}

func (b *builder) defineResponseCookieParameters() {
	cookiesParameterTypes, exist := b.hasParametersIn(responseCookieParametersGroup)
	if !exist {
		return
	}

	if len(cookiesParameterTypes) != 1 {
		b.errors = append(b.errors, InvalidMappingError(errors.New("supports only single response cookies service function return value")))
		return
	}

	b.responseCookieParameters = func(value reflect.Value) []*http.Cookie {
		if value.IsNil() {
			return nil
		}
		return value.Interface().([]*http.Cookie)
	}
}

func (b *builder) defineResponseErrorParameters() {
	responseErrorParameterTypes, exist := b.hasParametersIn(responseErrorParametersGroup)
	if !exist {
		return
	}

	if len(responseErrorParameterTypes) != 1 {
		b.errors = append(b.errors, InvalidMappingError(errors.New("mapping of multiple error values of service function return clause is not supported")))
		return
	}

	b.responseErrorParameters = DefaultErrorMapper
	if b.errorMapper != nil {
		b.responseErrorParameters = b.errorMapper
	}
}

func (b *builder) hasParametersIn(parametersGroup int) ([]reflect.Type, bool) {
	parameters, found := b.parametersBy[parametersGroup]
	return parameters, found && len(parameters) > 0
}

func (b builder) Encoder(encoder Encoder) Builder {
	cloned := b.clone()
	cloned.encoder = encoder
	return cloned
}

// TODO: how to put after interceptors?
// Would it be a traditional chain call?
// Do we want interceptors to be any kind of functions with same mapping rules that main service function apply to?
// Or just implement a specific interface?
func (b builder) After(interceptor Interceptor) Builder {
	cloned := b.clone()
	//cloned.after = interceptor
	return cloned
}

func (b builder) ErrorMapping(errorMapper ErrorMapper) Builder {
	cloned := b.clone()
	cloned.errorMapper = errorMapper
	return cloned
}

func (b builder) Build() EndpointProcessor {
	b.groupParameters(b.serviceValue.Type())
	b.defineProviders()
	if len(b.errors) > 0 {
		return EndpointProcessor{
			errors:         b.errors,
			processRequest: func(r *http.Request) ([]reflect.Value, error) { return nil, nil },
			produceResponse: func(executionResult []reflect.Value, executionError error, w http.ResponseWriter, r *http.Request) error {
				return nil
			},
		}
	}
	return EndpointProcessor{
		processRequest:  b.buildProcessRequest(),
		produceResponse: b.buildProduceResponse(),
	}
}

func (b *builder) buildProcessRequest() func(r *http.Request) ([]reflect.Value, error) {
	var valueCollectors []func(r *http.Request) ([]reflect.Value, error)

	if b.pathParameters != nil {
		valueCollectors = append(valueCollectors, func(r *http.Request) ([]reflect.Value, error) {
			return b.pathParameters(b.pathValues(r.URL.Path))
		})
	}

	for _, group := range b.orderOfOtherParameters {
		switch group {
		case headerParametersGroup:
			valueCollectors = append(valueCollectors, func(r *http.Request) ([]reflect.Value, error) {
				value, err := b.headerParameters(r.Header)
				return []reflect.Value{value}, err
			})

		case queryParametersGroup:
			valueCollectors = append(valueCollectors, func(r *http.Request) ([]reflect.Value, error) {
				value, err := b.queryParameters(r.URL.Query())
				return []reflect.Value{value}, err
			})

		case cookieParametersGroup:
			valueCollectors = append(valueCollectors, func(r *http.Request) ([]reflect.Value, error) {
				value, err := b.cookieParameters(r.Cookies())
				return []reflect.Value{value}, err
			})
		case bodyParametersGroup:
			valueCollectors = append(valueCollectors, func(r *http.Request) ([]reflect.Value, error) {
				value, err := b.bodyParameters(r.Body)
				return []reflect.Value{value}, err
			})
		}
	}

	return func(r *http.Request) ([]reflect.Value, error) {
		serviceValue := b.serviceValue
		var invokeValues []reflect.Value
		for _, valueCollector := range valueCollectors {
			values, err := valueCollector(r)
			if err != nil {
				return nil, err
			}
			invokeValues = append(invokeValues, values...)
		}
		return serviceValue.Call(invokeValues), nil
	}
}

func (b *builder) buildProduceResponse() func(executionResult []reflect.Value, executionError error, w http.ResponseWriter, r *http.Request) error {
	responseResolvers := map[int]func(results []reflect.Value, w http.ResponseWriter) error{
		responseStatusCodeParametersGroup: func(results []reflect.Value, w http.ResponseWriter) error {
			w.WriteHeader(http.StatusOK)
			return nil
		},
	}
	errorReturnValueIndex := -1

	for index, group := range b.orderOfResponseParameters {
		switch group {
		case responseHeaderParametersGroup:
			index := index
			responseResolvers[group] = func(results []reflect.Value, w http.ResponseWriter) error {
				headers := b.responseHeaderParameters(results[index])
				for header, values := range headers {
					if len(values) > 0 {
						w.Header().Set(header, values[0])
					}
					for _, value := range values {
						w.Header().Add(header, value[1:])
					}
				}
				return nil
			}

		case responseStatusCodeParametersGroup:
			index := index
			responseResolvers[group] = func(results []reflect.Value, w http.ResponseWriter) error {
				w.WriteHeader(b.responseStatusCodeParameters(results[index]))
				return nil
			}

		case responseCookieParametersGroup:
			index := index
			responseResolvers[group] = func(results []reflect.Value, w http.ResponseWriter) error {
				for _, cookieValue := range b.responseCookieParameters(results[index]) {
					http.SetCookie(w, cookieValue)
				}
				return nil
			}

		case responseBodyParametersGroup:
			index := index
			if b.encoder != nil {
				responseResolvers[group] = func(results []reflect.Value, w http.ResponseWriter) error {
					responseEntity := results[index]
					if responseEntity.Kind() == reflect.Ptr && responseEntity.IsNil() {
						return nil
					}
					return b.encoder(w)(responseEntity.Interface())
				}
				break
			}

			returnParameterType := b.parametersBy[group][0]
			switch returnParameterType.Kind() {
			case reflect.String:
				responseResolvers[group] = func(results []reflect.Value, w http.ResponseWriter) error {
					return b.encoder(w)(strings.NewReader(results[index].String()))
				}

			case reflect.Slice:
				responseResolvers[group] = func(results []reflect.Value, w http.ResponseWriter) error {
					return b.encoder(w)(bytes.NewReader(results[index].Interface().([]byte)))
				}

			case reflect.Array:
				responseResolvers[group] = func(results []reflect.Value, w http.ResponseWriter) error {
					responseEntityValue := results[index]
					length := responseEntityValue.Len()
					asSlice := make([]byte, length)
					for i := 0; i < length; i++ {
						asSlice[i] = byte(responseEntityValue.Index(i).Uint())
					}
					_, err := w.Write(asSlice)
					return err
				}
			}

		case responseErrorParametersGroup:
			errorReturnValueIndex = index
		}
	}

	if b.contentTypeProvider != nil {
		responseResolvers[responseContentTypeParametersGroup] = func(results []reflect.Value, w http.ResponseWriter) error {
			w.Header().Set("Content-Type", b.contentTypeProvider())
			return nil
		}
	}

	var parametersGroup []int
	for _, group := range [5]int{
		responseContentTypeParametersGroup,
		responseHeaderParametersGroup,
		responseCookieParametersGroup,
		responseStatusCodeParametersGroup,
		responseBodyParametersGroup,
	} {
		if _, found := responseResolvers[group]; found {
			parametersGroup = append(parametersGroup, group)
		}
	}

	defaultResponseProcessor := func(executionResult []reflect.Value, executionError error, w http.ResponseWriter, r *http.Request) error {
		for _, group := range parametersGroup {
			if err := responseResolvers[group](executionResult, w); err != nil {
				return err
			}
		}
		return nil
	}

	if errorReturnValueIndex == -1 {
		return defaultResponseProcessor
	} else {
		return func(executionResult []reflect.Value, executionError error, w http.ResponseWriter, r *http.Request) error {
			errorReturn := executionResult[errorReturnValueIndex].Interface()
			if errorReturn == nil {
				return defaultResponseProcessor(executionResult, executionError, w, r)
			}
			return b.responseErrorParameters(errorReturn.(error), w, r)
		}
	}
}

// TODO: do conversion of response params to HTTP response
// - body mapping is not implemented
// - error mapping: error -> StatusCode
// TODO: check parameters overflow in case it is possible
// TODO: Header parameters into user-defined types - ???
// maybe there will be some policy in naming those user-defined types
// TODO: make normal tests - visually check of prints is not good enough
