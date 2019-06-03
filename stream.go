package javaio

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"unsafe"
)

type StreamWriter struct {
	io.Writer
	handleMap map[unsafe.Pointer]refElem
}

type WriteObjecter interface {
	WriteObject(writer *StreamWriter) error
}

type Field struct {
	Name      string
	Typ       reflect.Type
	ClassName string
}

func NewStream(w io.Writer) (*StreamWriter, error) {
	stream := &StreamWriter{
		Writer: w,
	}
	if err := stream.writeHeader(); err != nil {
		return nil, err
	}
	return stream, nil
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
	return stream.writeRefOr(object, func() error {
		return stream.newObject(object)
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
	return nil
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
	return stream.writeRefOr(object, func() error {
		return stream.newClassDesc(object)
	})
}

func (stream *StreamWriter) writeUTF(s string) error {
	p := []byte(s)
	return stream.writeBinary(uint16(len(p)), p)
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
	stream.newHandle(object)
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
		f := v.Field(i)
		ft := f.Type()
		ftf := ft.Field(i)
		// Skip unexported fields.
		if ftf.PkgPath != "" {
			continue
		}
		// TODO: optimization.
		fieldName := ftf.Name
		javaClassName := className(reflect.New(ft).Interface())
		fields = append(fields, Field{
			Name:      fieldName,
			Typ:       ft,
			ClassName: javaClassName,
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

func (stream *StreamWriter) fieldDesc(field Field) error {
	var typeCode byte
	switch kind := field.Typ.Kind(); kind {
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
		return fmt.Errorf("fieldDesc: unsupported kind: %s", kind.String())
	}
	if err := stream.writeBinary(typeCode); err != nil {
		return err
	}
	if err := stream.writeUTF(field.Name); err != nil {
		return err
	}
	switch typeCode {
	case '[':

	case 'L':

	}
}
