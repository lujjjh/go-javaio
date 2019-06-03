package javaio

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"unicode"
	"unsafe"
)

type StreamWriter struct {
	io.Writer
	handleMap        map[unsafe.Pointer]refElem
	classNameHolders map[string]struct{}
}

type WriteObjecter interface {
	WriteObject(writer *StreamWriter) error
}

type Field struct {
	Name string
	Typ  reflect.Type
}

func NewStreamWriter(w io.Writer) (*StreamWriter, error) {
	stream := &StreamWriter{
		Writer:           w,
		handleMap:        map[unsafe.Pointer]refElem{},
		classNameHolders: make(map[string]struct{}),
	}
	if err := stream.writeHeader(); err != nil {
		return nil, err
	}
	return stream, nil
}

func (stream *StreamWriter) classNameHolder(className string) struct{} {
	holder, ok := stream.classNameHolders[className]
	if !ok {
		holder = struct{}{}
		stream.classNameHolders[className] = holder
	}
	return holder
}

func (stream *StreamWriter) writeBinary(values ...interface{}) error {
	for _, value := range values {
		if err := binary.Write(stream, binary.BigEndian, value); err != nil {
			return err
		}
	}
	return nil
}

func (stream *StreamWriter) writeHeader() error {
	return stream.writeBinary(StreamMagic, StreamVersion)
}

func (stream *StreamWriter) writeRefOr(object interface{}, f func() error) error {
	if handle := stream.findHandle(object); handle != -1 {
		return stream.writeBinary(TcReference, handle)
	}
	return f()
}

func (stream *StreamWriter) writeObject(object interface{}) error {
	v := unpackPointer(reflect.ValueOf(object))
	if !v.IsValid() {
		return stream.writeBinary(TcNull)
	}
	code := typeCode(unpackPointerType(v.Type()))
	if code != '[' && code != 'L' {
		return stream.writeBinary(v.Interface())
	}
	return stream.writeRefOr(object, func() error {
		switch code {
		case 'L':
			return stream.newObject(object)
		case '[':
			return stream.newArray(object)
		default:
			return fmt.Errorf("cannot serialize type: %T", object)
		}
	})
}

func (stream *StreamWriter) newObject(object interface{}) error {
	if err := stream.writeBinary(TcObject); err != nil {
		return err
	}
	if err := stream.classDesc(object); err != nil {
		return err
	}
	stream.newHandle(object)
	return stream.classData(object)
}

func super(object interface{}) interface{} {
	type Superer interface {
		Super() interface{}
	}
	if superer, haveSuperer := object.(Superer); haveSuperer {
		return superer.Super()
	}
	return nil
}

func (stream *StreamWriter) classDesc(object interface{}) error {
	if object == nil {
		return stream.writeBinary(TcNull)
	}
	return stream.writeRefOr(stream.classNameHolder(className(object)), func() error {
		return stream.newClassDesc(object)
	})
}

func (stream *StreamWriter) writeUTF(s string) error {
	p := []byte(s)
	return stream.writeBinary(uint16(len(p)), p)
}

func (stream *StreamWriter) writeLongUTF(s string) error {
	p := []byte(s)
	return stream.writeBinary(uint64(len(p)), p)
}

func className(object interface{}) string {
	type ClassNamer interface {
		ClassName() string
	}
	if classNamer, haveClassNamer := object.(ClassNamer); haveClassNamer {
		return classNamer.ClassName()
	}
	return ""
}

func serialVersionUID(object interface{}) int64 {
	type SerialVersionUIDer interface {
		SerialVersionUID() int64
	}
	if serialVersionUIDer, haveSerialVersionUIDer := object.(SerialVersionUIDer); haveSerialVersionUIDer {
		return serialVersionUIDer.SerialVersionUID()
	}
	return 0
}

func (stream *StreamWriter) newClassDesc(object interface{}) error {
	if err := stream.writeBinary(TcClassdesc); err != nil {
		return err
	}
	if err := stream.writeUTF(className(object)); err != nil {
		return err
	}
	if err := stream.writeBinary(serialVersionUID(object)); err != nil {
		return err
	}
	stream.newHandle(stream.classNameHolder(className(object)))
	if err := stream.classDescInfo(object); err != nil {
		return err
	}
	return nil
}

func writeObjecter(object interface{}) WriteObjecter {
	if writeObjecter, haveWriteObjecter := object.(WriteObjecter); haveWriteObjecter {
		return writeObjecter
	}
	return nil
}

func classDescFlags(object interface{}) (flags byte) {
	flags |= ScSerializable
	if writeObjecter(object) != nil {
		flags |= ScWriteMethod
	}
	if sup := super(object); sup != nil {
		flags |= classDescFlags(sup)
	}
	// TODO: Enum?...
	return flags
}

func (stream *StreamWriter) classDescInfo(object interface{}) error {
	if err := stream.writeBinary(classDescFlags(object)); err != nil {
		return err
	}
	if err := stream.fields(object); err != nil {
		return err
	}
	if err := stream.classAnnotation(object); err != nil {
		return err
	}
	if err := stream.superClassDesc(object); err != nil {
		return err
	}
	return nil
}

func (stream *StreamWriter) fields(object interface{}) error {
	v := unpackPointer(reflect.ValueOf(object))
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("fields: object must be a struct")
	}
	numField := v.NumField()
	fields := make([]Field, 0, numField)
	for i := 0; i < numField; i++ {
		t := v.Type()
		tf := t.Field(i)
		// Skip unexported fields.
		if tf.PkgPath != "" {
			continue
		}
		// TODO: optimization.
		fieldName := lowerCamelCase(tf.Name)
		fields = append(fields, Field{
			Name: fieldName,
			Typ:  unpackPointerType(tf.Type),
		})
	}
	if err := stream.writeBinary(int16(len(fields))); err != nil {
		return err
	}
	for _, field := range fields {
		if err := stream.fieldDesc(field); err != nil {
			return err
		}
	}
	return nil
}

func lowerCamelCase(name string) string {
	runes := []rune(name)
	if len(runes) > 0 {
		runes[0] = unicode.ToLower(runes[0])
	}
	return string(runes)
}

func (stream *StreamWriter) fieldDesc(field Field) error {
	typeCode := typeCode(field.Typ)
	if err := stream.writeBinary(typeCode); err != nil {
		return err
	}
	if err := stream.writeUTF(field.Name); err != nil {
		return err
	}
	switch typeCode {
	case 'L', '[':
		return stream.writeString(fieldDescriptor(field.Typ))
	}
	return nil
}

func (stream *StreamWriter) classAnnotation(object interface{}) error {
	// TODO
	return stream.writeBinary(TcEndblockdata)
}

func (stream *StreamWriter) superClassDesc(object interface{}) error {
	return stream.classDesc(super(object))
}

func (stream *StreamWriter) writeString(s string) error {
	return stream.writeRefOr(s, func() error {
		p := []byte(s)
		l := len(p)
		if l <= 0xFFFF {
			if err := stream.writeBinary(TcString); err != nil {
				return err
			}
			stream.newHandle(s)
			return stream.writeUTF(s)
		}
		if err := stream.writeBinary(TcLongstring); err != nil {
			return err
		}
		stream.newHandle(s)
		return stream.writeLongUTF(s)
	})
}

func (stream *StreamWriter) classData(object interface{}) error {
	flags := classDescFlags(object)
	if flags&ScSerializable != 0 {
		if flags&ScWriteMethod == 0 {
			return stream.nowrclass(object)
		}
	}
	return fmt.Errorf("classData: flags %d not supported", int(flags))
}

func (stream *StreamWriter) nowrclass(object interface{}) error {
	v := unpackPointer(reflect.ValueOf(object))
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("fields: object must be a struct")
	}
	numField := v.NumField()
	for i := 0; i < numField; i++ {
		t := v.Type()
		tf := t.Field(i)
		// Skip unexported fields.
		if tf.PkgPath != "" {
			continue
		}
		f := v.Field(i)
		if err := stream.writeObject(f.Interface()); err != nil {
			return err
		}
	}
	return nil
}

func (stream *StreamWriter) newArray(object interface{}) error {
	array := NewArray(object)
	if err := stream.writeBinary(TcArray); err != nil {
		return err
	}
	if err := stream.classDesc(array); err != nil {
		return err
	}
	stream.newHandle(object)
	l := array.Len()
	if err := stream.writeBinary(int32(l)); err != nil {
		return err
	}
	for i := 0; i < l; i++ {
		if err := stream.writeObject(array.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

func typeCode(typ reflect.Type) byte {
	var typeCode byte
	switch kind := typ.Kind(); kind {
	case reflect.Uint8:
		typeCode = 'B'
	// TODO: char?
	case reflect.Float64:
		typeCode = 'D'
	case reflect.Float32:
		typeCode = 'F'
	case reflect.Int32, reflect.Uint32:
		typeCode = 'I'
	case reflect.Int64, reflect.Uint64:
		typeCode = 'J'
	case reflect.Int16, reflect.Uint16:
		typeCode = 'S'
	case reflect.Bool:
		typeCode = 'Z'
	case reflect.Array, reflect.Slice:
		typeCode = '['
	case reflect.Struct:
		typeCode = 'L'
	default:
	}
	return typeCode
}

func fieldDescriptor(typ reflect.Type) string {
	code := string(typeCode(typ))
	switch code {
	case "L":
		return code + classNameFromTyp(typ) + ";"
	case "[":
		return code + fieldDescriptor(unpackPointerType(typ.Elem()))
	}
	return code
}

func classNameFromTyp(typ reflect.Type) string {
	return className(reflect.New(typ).Interface())
}
