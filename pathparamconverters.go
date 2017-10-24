package main

import (
	"reflect"
	"strconv"
)

type PathParameterConverter interface {
	Convert(pathPart string) (reflect.Value, error)
}

var PathParameterConverterType = reflect.TypeOf((*PathParameterConverter)(nil)).Elem()

type StringPathParameterConverter struct{}

func (sc StringPathParameterConverter) Convert(pathPart string) (reflect.Value, error) {
	return reflect.ValueOf(pathPart), nil
}

var stringPathParameterConverterSingleton = StringPathParameterConverter{}

type IntPathParameterConverter struct {
	bitSize int
	valueOf func(d interface{}) reflect.Value
}

func (ic IntPathParameterConverter) Convert(pathPart string) (reflect.Value, error) {
	parsed, err := strconv.ParseInt(pathPart, 10, ic.bitSize)
	if err != nil {
		return reflect.Value{}, err
	}
	return ic.valueOf(parsed), nil
}

type UintPathParameterConverter struct {
	bitSize int
	valueOf func(d interface{}) reflect.Value
}

func (uc UintPathParameterConverter) Convert(pathPart string) (reflect.Value, error) {
	parsed, err := strconv.ParseUint(pathPart, 10, uc.bitSize)
	if err != nil {
		return reflect.Value{}, err
	}
	return uc.valueOf(parsed), nil
}

type BoolPathParameterConverter struct{}

func (bc BoolPathParameterConverter) Convert(pathPart string) (reflect.Value, error) {
	parsed, err := strconv.ParseBool(pathPart)
	if err != nil {
		return reflect.Value{}, err
	}
	return reflect.ValueOf(bool(parsed)), nil
}

var boolPathParameterConverterSingleton = BoolPathParameterConverter{}

type SliceBytePathParameterConverter struct{}

func (sbc SliceBytePathParameterConverter) Convert(pathPart string) (reflect.Value, error) {
	return reflect.ValueOf([]byte(pathPart)), nil
}

var sliceBytePathParameterConverterSingleton = SliceBytePathParameterConverter{}

type ArrayBytePathParameterConverter struct {
	length      int
	elementType reflect.Type
}

func (abc ArrayBytePathParameterConverter) Convert(pathPart string) (reflect.Value, error) {
	arrayType := reflect.ArrayOf(abc.length, abc.elementType)
	arrayValuePtr := reflect.New(arrayType)
	arrayValue := arrayValuePtr.Elem()
	reflect.Copy(arrayValue, reflect.ValueOf(pathPart))
	return arrayValue, nil
}
