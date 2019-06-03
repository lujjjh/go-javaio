package javaio

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"reflect"
)

type Array struct {
	value reflect.Value
}

func NewArray(x interface{}) *Array {
	value := reflect.ValueOf(x)
	if !value.IsValid() {
		panic("NewArray: value must be valid")
	}
	if kind := value.Kind(); kind != reflect.Array && kind != reflect.Slice {
		panic("NewArray: value must be an array or a slice")
	}
	return &Array{
		value: value,
	}
}

func (array *Array) ClassName() string {
	return fieldDescriptor(array.value.Type())
}

func (array *Array) SerialVersionUID() int64 {
	var buf bytes.Buffer
	streamWriter, err := NewStreamWriter(&buf)
	if err != nil {
		return 0
	}
	_ = streamWriter.writeUTF(array.ClassName())
	_ = streamWriter.writeBinary(int32(1 | 16 | 1024)) // Modifier.PUBLIC | Modifier.FINAL | Modifier.ABSTRACT
	hashBytes := sha1.Sum(buf.Bytes()[4:])
	return int64(binary.LittleEndian.Uint64(hashBytes[:8]))
}

func (array *Array) Len() int {
	return array.value.Len()
}

func (array *Array) Index(i int) interface{} {
	value := array.value.Index(i)
	if !value.CanInterface() {
		return nil
	}
	return value.Interface()
}
