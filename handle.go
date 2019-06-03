package javaio

import (
	"reflect"
	"unsafe"
)

type refElem struct {
	kind  reflect.Kind
	index int32
}

func kindAndPointer(v reflect.Value) (reflect.Kind, unsafe.Pointer) {
	if v.Kind() == reflect.Ptr {
		for v.Elem().Kind() == reflect.Ptr {
			v = v.Elem()
		}
		switch kind := v.Kind(); kind {
		case reflect.Slice, reflect.Map:
			return kind, unsafe.Pointer(v.Pointer())
		default:
			return kind, unsafe.Pointer(packPointer(v).Pointer())
		}
	}
	switch kind := v.Kind(); kind {
	case reflect.Slice, reflect.Map:
		return kind, unsafe.Pointer(v.Pointer())
	default:
		return kind, unsafe.Pointer(packPointer(v).Pointer())
	}
}

func unpackPointer(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v
}

func packPointer(x reflect.Value) reflect.Value {
	v := reflect.New(x.Type())
	v.Elem().Set(x)
	return v
}

func unpackPointerType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func (stream *StreamWriter) newHandle(object interface{}) {
	v := reflect.ValueOf(object)
	kind, pointer := kindAndPointer(v)
	index := baseWireHandle + int32(len(stream.handleMap))
	stream.handleMap[pointer] = refElem{kind, index}
}

func (stream *StreamWriter) findHandle(object interface{}) int32 {
	v := reflect.ValueOf(object)
	kind, pointer := kindAndPointer(v)
	if elem, ok := stream.handleMap[pointer]; ok {
		if elem.kind == kind {
			return elem.index
		}
	}
	return -1
}
