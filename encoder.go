package javaio

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"
	"unicode"
	"unsafe"
)

type classNameHolder struct {
	ClassName string
}

type Encoder struct {
	w                  io.Writer
	handleMap          map[unsafe.Pointer]refElem
	classNameHolders   map[string]*classNameHolder
	stringHolders      map[string]*string
	blockDataMode      bool
	blockDataBuffer    [1024]byte
	blockDataBufferPos int
}

type ObjectWriter interface {
	WriteObject(enc *Encoder) error
}

type Field struct {
	Name  string
	Typ   reflect.Type
	Value reflect.Value
}

func NewEncoder(w io.Writer) (*Encoder, error) {
	stream := &Encoder{
		w:                w,
		handleMap:        map[unsafe.Pointer]refElem{},
		classNameHolders: make(map[string]*classNameHolder),
		stringHolders:    make(map[string]*string),
	}
	if err := stream.writeHeader(); err != nil {
		return nil, err
	}
	return stream, nil
}

func (enc *Encoder) Write(p []byte) (int, error) {
	if !enc.blockDataMode {
		return enc.w.Write(p)
	}
	l := len(p)
	for l > 0 {
		if enc.blockDataBufferPos >= len(enc.blockDataBuffer) {
			if err := enc.flush(); err != nil {
				return 0, err
			}
		}
		wLen := l
		if maxWLen := len(enc.blockDataBuffer) - enc.blockDataBufferPos; wLen > maxWLen {
			wLen = maxWLen
		}
		copy(enc.blockDataBuffer[enc.blockDataBufferPos:], p[:wLen])
		enc.blockDataBufferPos += wLen
		l -= wLen
	}
	return len(p), nil
}

func (enc *Encoder) blockDataModeOn() {
	enc.blockDataMode = true
}

func (enc *Encoder) blockDataModeOffAndFlush() error {
	if err := enc.flush(); err != nil {
		return err
	}
	enc.blockDataMode = false
	return nil
}

func (enc *Encoder) flush() error {
	if !enc.blockDataMode {
		return nil
	}
	if enc.blockDataBufferPos == 0 {
		return nil
	}
	if err := enc.writeBlockHeader(enc.blockDataBufferPos); err != nil {
		return err
	}
	_, err := enc.w.Write(enc.blockDataBuffer[:enc.blockDataBufferPos])
	if err != nil {
		return err
	}
	enc.blockDataBufferPos = 0
	return nil
}

func (enc *Encoder) writeBlockHeader(i int) error {
	if i <= 0xFF {
		_, err := enc.w.Write([]byte{TcBlockdata, byte(i)})
		return err
	}
	if _, err := enc.w.Write([]byte{TcBlockdatalong}); err != nil {
		return err
	}
	return binary.Write(enc.w, binary.BigEndian, int32(i))
}

func (enc *Encoder) WriteObject(object interface{}) error {
	return enc.writeObject(object)
}

func (enc *Encoder) classNameHolder(className string) *classNameHolder {
	holder, ok := enc.classNameHolders[className]
	if !ok {
		holder = &classNameHolder{
			ClassName: className,
		}
		enc.classNameHolders[className] = holder
	}
	return holder
}

func (enc *Encoder) stringHolder(s string) *string {
	holder, ok := enc.stringHolders[s]
	if !ok {
		holder = &s
		enc.stringHolders[s] = holder
	}
	return holder
}

func (enc *Encoder) writeBinary(values ...interface{}) error {
	for _, value := range values {
		if err := binary.Write(enc, binary.BigEndian, value); err != nil {
			return err
		}
	}
	return nil
}

func (enc *Encoder) writeHeader() error {
	return enc.writeBinary(StreamMagic, StreamVersion)
}

func (enc *Encoder) writeRefOr(object interface{}, f func() error) error {
	if handle := enc.findHandle(object); handle != -1 {
		return enc.writeBinary(TcReference, handle)
	}
	return f()
}

func (enc *Encoder) writeObject(object interface{}) error {
	v := unpackPointer(reflect.ValueOf(object))
	if !v.IsValid() {
		return enc.writeBinary(TcNull)
	}
	code := typeCode(unpackPointerType(v.Type()))
	if code != '[' && code != 'L' {
		return enc.writeBinary(v.Interface())
	}

	oldBlockDataMode := enc.blockDataMode
	if err := enc.blockDataModeOffAndFlush(); err != nil {
		return err
	}
	defer func() {
		if oldBlockDataMode {
			enc.blockDataModeOn()
		}
	}()
	if v, ok := object.(*Serializable); ok {
		object = v.Value
	}
	if v, ok := object.(*String); ok {
		object = v.Value
	}
	if v, ok := object.(string); ok {
		return enc.writeString(v)
	}
	return enc.writeRefOr(object, func() error {
		switch code {
		case 'L':
			return enc.newObject(object)
		case '[':
			return enc.newArray(object)
		default:
			return fmt.Errorf("cannot serialize type: %T", object)
		}
	})
}

func (enc *Encoder) newObject(object interface{}) error {
	if array, ok := object.(*Array); ok {
		if err := enc.writeBinary(TcArray); err != nil {
			return err
		}
		if err := enc.writeBinary(TcClassdesc); err != nil {
			return err
		}

		if err := enc.writeClassDesc(array); err != nil {
			return err
		}
		if err := enc.writeBinary(serialVersionUID(object)); err != nil {
			return err
		}
		if err := enc.classDescInfo(object); err != nil {
			return err
		}
		enc.newHandle(object)
		l := array.Len()
		if err := enc.writeBinary(int32(l)); err != nil {
			return err
		}
		for i := 0; i < l; i++ {
			if err := enc.writeObject(array.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := enc.writeBinary(TcObject); err != nil {
		return err
	}
	if err := enc.classDesc(object); err != nil {
		return err
	}
	enc.newHandle(object)
	return enc.classData(object)
}

func (enc *Encoder) writeClassDesc(array *Array) error {
	if array.Len() < 1 {
		return enc.writeBinary(TcNull)
	}
	return enc.writeUTF("[L" + classNameFromTyp(unpackPointerType(reflect.TypeOf(array.Index(0)))) + ";")
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

func (enc *Encoder) classDesc(object interface{}) error {
	if object == nil {
		return enc.writeBinary(TcNull)
	}
	return enc.writeRefOr(enc.classNameHolder(className(object)), func() error {
		return enc.newClassDesc(object)
	})
}

func (enc *Encoder) writeUTF(s string) error {
	p := []byte(s)
	return enc.writeBinary(uint16(len(p)), p)
}

func (enc *Encoder) writeLongUTF(s string) error {
	p := []byte(s)
	return enc.writeBinary(uint64(len(p)), p)
}

func className(object interface{}) string {
	type ClassNamer interface {
		ClassName() string
	}
	if classNamer, haveClassNamer := object.(ClassNamer); haveClassNamer {
		return classNamer.ClassName()
	}
	log.Panicf("object %v does not implement ClassNamer", object)
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

func (enc *Encoder) newClassDesc(object interface{}) error {
	if err := enc.writeBinary(TcClassdesc); err != nil {
		return err
	}
	if err := enc.writeUTF(className(object)); err != nil {
		return err
	}
	if err := enc.writeBinary(serialVersionUID(object)); err != nil {
		return err
	}
	enc.newHandle(enc.classNameHolder(className(object)))
	if err := enc.classDescInfo(object); err != nil {
		return err
	}
	return nil
}

func writeObjecter(object interface{}) ObjectWriter {
	if writeObjecter, haveWriteObjecter := object.(ObjectWriter); haveWriteObjecter {
		return writeObjecter
	}
	return nil
}

func classDescFlags(object interface{}) (flags byte) {
	flags |= ScSerializable
	if writeObjecter(object) != nil {
		flags |= ScWriteMethod
	}
	//if sup := super(object); sup != nil {
	//	flags |= classDescFlags(sup)
	//}
	// TODO: Enum?...
	return flags
}

func (enc *Encoder) classDescInfo(object interface{}) error {
	if err := enc.writeBinary(classDescFlags(object)); err != nil {
		return err
	}
	if err := enc.fields(object); err != nil {
		return err
	}
	if err := enc.classAnnotation(object); err != nil {
		return err
	}
	if err := enc.superClassDesc(object); err != nil {
		return err
	}
	return nil
}

func (enc *Encoder) fields(object interface{}) error {
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
		var fieldName string
		if tag := tf.Tag.Get("javaio"); tag != "" {
			if tag == "-" {
				continue
			}
			fieldName = tag
		} else {
			fieldName = lowerCamelCase(tf.Name)
		}
		fields = append(fields, Field{
			Name:  fieldName,
			Typ:   unpackPointerType(tf.Type),
			Value: v.Field(i),
		})
	}
	if err := enc.writeBinary(int16(len(fields))); err != nil {
		return err
	}

	enc.sort(fields)
	for _, field := range fields {
		if err := enc.fieldDesc(field); err != nil {
			return err
		}
	}
	return nil
}

func (enc *Encoder) sort(fields []Field) {
	len := len(fields)
	if len < 2 {
		return
	}
	if len >= 32 {
		return
	}
	index := enc.compare(fields)
	var pri Field
	for ; index < len; index++ {
		left := 0
		right := index
		mid := 0
		pivot := fields[index]
		for left < right {
			mid = (left + right) >> 1
			if !enc.compireTo(pivot, fields[mid]) {
				right = mid
				continue
			}
			left = mid + 1
		}

		n := index - left
		switch n {
		case 2:
			fields[left+2] = fields[left+1]
			fields[left+1] = fields[left]
		case 1:
			fields[left+1] = fields[left]
		default:
			var tmp Field
			for i := 1; i <= n; i++ {
				if pri.Typ == nil {
					pri = fields[left+i]
					fields[left+i] = fields[left+i-1]
					continue
				}
				tmp = fields[left+i]
				fields[left+i] = pri
				pri = tmp
			}
		}
		fields[left] = pivot
	}
}

func (enc *Encoder) compare(fields []Field) int {
	index := 1
	for ; index < len(fields); index++ {
		if !enc.compireTo(fields[index], fields[index-1]) {
			break
		}
	}
	if index == 0 {
		index++
	}
	return index
}

func (enc *Encoder) compireTo(now, pri Field) bool {
	code := now.Typ.Kind()
	lastCode := pri.Typ.Kind()
	primitive := code >= reflect.Bool && code <= reflect.Uint64 || code == reflect.Float32 || code == reflect.Float64
	lastType := lastCode >= reflect.Bool && lastCode <= reflect.Uint64 || lastCode == reflect.Float32 || lastCode == reflect.Float64

	if lastType == primitive {
		return now.Name > pri.Name
	}
	return !primitive
}

func lowerCamelCase(name string) string {
	runes := []rune(name)
	if len(runes) > 0 {
		runes[0] = unicode.ToLower(runes[0])
	}
	return string(runes)
}

func (enc *Encoder) fieldDesc(field Field) error {
	if field.Typ.Kind() == reflect.Interface {
		field.Typ = unpackPointerType(reflect.TypeOf(field.Value.Interface()))
	}
	typeCode := typeCode(field.Typ)
	array, ok := field.Value.Interface().(*Array)
	if ok {
		typeCode = '['
	}
	if err := enc.writeBinary(typeCode); err != nil {
		return err
	}
	if err := enc.writeUTF(field.Name); err != nil {
		return err
	}
	switch typeCode {
	case 'L', '[':
		if array != nil {
			return enc.writeString(array.ClassName())
		}
		return enc.writeString(fieldDescriptor(field.Typ))
	}
	return nil
}

func (enc *Encoder) classAnnotation(object interface{}) error {
	// TODO
	return enc.writeBinary(TcEndblockdata)
}

func (enc *Encoder) superClassDesc(object interface{}) error {
	return enc.classDesc(super(object))
}

func (enc *Encoder) writeString(s string) error {
	holder := enc.stringHolder(s)
	return enc.writeRefOr(holder, func() error {
		p := []byte(s)
		l := len(p)
		if l <= 0xFFFF {
			if err := enc.writeBinary(TcString); err != nil {
				return err
			}
			enc.newHandle(holder)
			return enc.writeUTF(s)
		}
		if err := enc.writeBinary(TcLongstring); err != nil {
			return err
		}
		enc.newHandle(s)
		return enc.writeLongUTF(s)
	})
}

func (enc *Encoder) classData(object interface{}) error {
	if sup := super(object); sup != nil {
		if err := enc.classData(sup); err != nil {
			return err
		}
	}
	flags := classDescFlags(object)
	if flags&ScSerializable != 0 {
		if flags&ScWriteMethod == 0 {
			return enc.nowrclass(object)
		} else {
			enc.blockDataModeOn()
			err := writeObjecter(object).WriteObject(enc)
			if err != nil {
				return err
			}
			if err := enc.blockDataModeOffAndFlush(); err != nil {
				return err
			}
			return enc.writeBinary(TcEndblockdata)
		}
	}
	return fmt.Errorf("classData: flags %d not supported", int(flags))
}

func (enc *Encoder) nowrclass(object interface{}) error {
	v := unpackPointer(reflect.ValueOf(object))
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("fields: object must be a struct")
	}
	numField := v.NumField()
	t := v.Type()
	fields := make([]Field, 0, numField)
	for i := 0; i < numField; i++ {
		tf := t.Field(i)
		// Skip unexported fields.
		if tf.PkgPath != "" {
			continue
		}
		if tag := tf.Tag.Get("javaio"); tag == "-" {
			continue
		}
		fields = append(fields, Field{
			Name:  lowerCamelCase(tf.Name),
			Typ:   unpackPointerType(tf.Type),
			Value: v.Field(i),
		})
	}
	enc.sort(fields)
	for _, f := range fields {
		if err := enc.writeObject(f.Value.Interface()); err != nil {
			return err
		}
	}
	return nil
}

func (enc *Encoder) newArray(object interface{}) error {
	array := NewArray(object)
	if err := enc.writeBinary(TcArray); err != nil {
		return err
	}
	if err := enc.classDesc(array); err != nil {
		return err
	}
	enc.newHandle(object)
	l := array.Len()
	if err := enc.writeBinary(int32(l)); err != nil {
		return err
	}
	for i := 0; i < l; i++ {
		if err := enc.writeObject(array.Index(i)); err != nil {
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
	case reflect.Struct, reflect.String:
		typeCode = 'L'
	default:
		log.Panicf("unsupported type: %v", typ)
	}
	return typeCode
}

func fieldDescriptor(typ reflect.Type) string {
	code := string(typeCode(typ))
	switch code {
	case "L":
		return code + strings.ReplaceAll(classNameFromTyp(typ), ".", "/") + ";"
	case "[":
		return code + fieldDescriptor(unpackPointerType(typ.Elem()))
	}
	return code
}

func classNameFromTyp(typ reflect.Type) string {
	return className(reflect.New(typ).Interface())
}
