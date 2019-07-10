package javaio

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

type Decoder struct {
	r             *bufio.Reader
	typs          map[string]reflect.Type
	handles       []interface{}
	blockDataMode bool
	unread        int
}

type classDesc struct {
	name             string
	serialVersionUID int64
	info             classDescInfo
}

type classDescInfo struct {
	flags          byte
	fields         []fieldDesc
	superClassDesc *classDesc
}

type fieldDesc struct {
	typeCode  byte
	name      string
	className string
}

func NewDecoder(r io.Reader) (*Decoder, error) {
	dec := &Decoder{
		r:    bufio.NewReader(r),
		typs: make(map[string]reflect.Type),
	}
	if err := dec.readHeader(); err != nil {
		return nil, err
	}
	dec.blockDataMode = true
	return dec, nil
}

func (dec *Decoder) RegisterType(name string, typ reflect.Type) {
	dec.typs[name] = typ
}

func (dec *Decoder) Read(p []byte) (int, error) {
	if !dec.blockDataMode {
		return dec.r.Read(p)
	}
	if len(p) > dec.unread {
		return 0, errors.New("read out of block data")
	}
	n, err := dec.r.Read(p)
	dec.unread -= n
	if dec.unread <= 0 {
		dec.blockDataMode = false
	}
	return n, err
}

func (dec *Decoder) readBinary(dsts ...interface{}) error {
	for _, dst := range dsts {
		if err := binary.Read(dec.r, binary.BigEndian, dst); err != nil {
			return err
		}
	}
	return nil
}

func (dec *Decoder) readHeader() error {
	var (
		magic   uint16
		version int16
	)
	if err := dec.readBinary(&magic, &version); err != nil {
		return err
	}
	if magic != StreamMagic {
		return errors.New("readHeader: invalid stream header")
	}
	if version != StreamVersion {
		return errors.New("readHeader: unsupported stream version")
	}
	return nil
}

func (dec *Decoder) ReadObject() (interface{}, error) {
	return dec.readObject()
}

func (dec *Decoder) readObject() (interface{}, error) {
	tc, err := dec.r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch tc {
	case TcNull:
		return nil, nil
	case TcReference:
		return dec.readHandle()
	case TcString, TcLongstring:
		if err := dec.r.UnreadByte(); err != nil {
			return nil, err
		}
		s, err := dec.readString()
		if err != nil {
			return nil, err
		}
		return s, nil
	case TcArray:
		return dec.readArray()
	case TcObject:
		return dec.readOrdinaryObject()
	default:
		return "", fmt.Errorf("readObject: invalid type code: %02X", tc)
	}
}

func (dec *Decoder) assignHandle(v interface{}) int {
	dec.handles = append(dec.handles, v)
	return len(dec.handles) - 1
}

func (dec *Decoder) readUTF() (string, error) {
	var l uint16
	if err := dec.readBinary(&l); err != nil {
		return "", err
	}
	p := make([]byte, l)
	if err := dec.readBinary(p); err != nil {
		return "", err
	}
	return string(p), nil
}

func (dec *Decoder) readLongUTF() (string, error) {
	var l uint64
	if err := dec.readBinary(l); err != nil {
		return "", err
	}
	p := make([]byte, l)
	if err := dec.readBinary(p); err != nil {
		return "", err
	}
	return string(p), nil
}

func (dec *Decoder) readHandle() (interface{}, error) {
	var handle int32
	if err := dec.readBinary(&handle); err != nil {
		return nil, err
	}
	handle -= baseWireHandle
	if handle < 0 || int(handle) >= len(dec.handles) {
		return nil, fmt.Errorf("invalid handle value: %d", handle+baseWireHandle)
	}
	return dec.handles[handle], nil
}

func (dec *Decoder) readString() (string, error) {
	tc, err := dec.r.ReadByte()
	if err != nil {
		return "", err
	}
	switch tc {
	case TcReference:
		v, err := dec.readHandle()
		if err != nil {
			return "", err
		}
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("readString: reference is not a string")
		}
		return s, nil
	case TcString:
		s, err := dec.readUTF()
		if err != nil {
			return "", nil
		}
		dec.assignHandle(s)
		return s, nil
	case TcLongstring:
		s, err := dec.readLongUTF()
		if err != nil {
			return "", nil
		}
		dec.assignHandle(s)
		return s, nil
	default:
		return "", fmt.Errorf("readString: invalid type code: %02X", tc)
	}
}

func (dec *Decoder) readClassDesc() (*classDesc, error) {
	tc, err := dec.r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch tc {
	case TcNull:
		return nil, nil
	case TcReference:
		v, err := dec.readHandle()
		if err != nil {
			return nil, err
		}
		desc, ok := v.(*classDesc)
		if !ok {
			return nil, fmt.Errorf("readClassDesc: reference is not a classDesc")
		}
		return desc, nil
	case TcProxyclassdesc:
		return nil, errors.New("readClassDesc: proxy class not implemented")
	case TcClassdesc:
		return dec.readNonProxyDesc()
	default:
		return nil, fmt.Errorf("readClassDesc: invalid type code: %02X", tc)
	}
}

func (dec *Decoder) readNonProxyDesc() (*classDesc, error) {
	desc := &classDesc{}
	dec.assignHandle(desc)
	if err := dec.readClassDescriptor(desc); err != nil {
		return nil, err
	}
	superClassDesc, err := dec.readClassDesc()
	if err != nil {
		return nil, err
	}
	desc.info.superClassDesc = superClassDesc
	return desc, nil
}

func (dec *Decoder) readClassDescriptor(desc *classDesc) error {
	name, err := dec.readUTF()
	if err != nil {
		return err
	}
	desc.name = name
	var suid int64
	var flags byte
	if err := dec.readBinary(&suid, &flags); err != nil {
		return err
	}
	desc.serialVersionUID = suid
	if flags&ScEnum != 0 {
		// TODO: support enum
		return errors.New("readNonProxyDesc: does not support enum")
	}

	var numFields int16
	if err := dec.readBinary(&numFields); err != nil {
		return err
	}
	fields := make([]fieldDesc, 0, int(numFields))
	for i := 0; i < int(numFields); i++ {
		var tcode byte
		if err := dec.readBinary(&tcode); err != nil {
			return err
		}
		fname, err := dec.readUTF()
		if err != nil {
			return err
		}
		var className string
		if tcode == 'L' || tcode == '[' {
			className, err = dec.readString()
			if err != nil {
				return err
			}
		}
		fields = append(fields, fieldDesc{
			typeCode:  tcode,
			name:      fname,
			className: className,
		})
	}
	desc.info.fields = fields

	tc, err := dec.r.ReadByte()
	if err != nil {
		return err
	}
	if tc != TcEndblockdata {
		return fmt.Errorf("readClassDescriptor: expected TC_ENDBLOCKDATA, got %02X", tc)
	}

	return nil
}

func (dec *Decoder) readArray() (interface{}, error) {
	desc, err := dec.readClassDesc()
	if err != nil {
		return nil, err
	}
	array := &Array{}
	dec.assignHandle(array)
	var l int32
	if err := dec.readBinary(&l); err != nil {
		return nil, err
	}

	typ, err := dec.typFromFieldDescriptor(desc.name)
	if err != nil {
		return nil, err
	}
	if typ.Kind() != reflect.Slice {
		return nil, fmt.Errorf("readArray: expected slice, got '%s'", typ.Kind())
	}
	array.value = reflect.MakeSlice(typ.Elem(), int(l), int(l))
	for i := 0; i < int(l); i++ {
		data, err := dec.readObject()
		if err != nil {
			return nil, err
		}
		dataVal := reflect.ValueOf(data)
		elem := array.value.Index(i)
		if !dataVal.Type().AssignableTo(elem.Type()) {
			return nil, fmt.Errorf("readArray: type %s is not assignable to type %s", dataVal.Type(), elem.Type())
		}
		elem.Set(dataVal)
	}
	return array, nil
}

func (dec *Decoder) readOrdinaryObject() (interface{}, error) {
	desc, err := dec.readClassDesc()
	if err != nil {
		return nil, err
	}
	typ, err := dec.getTypeFromClassName(desc.name)
	if err != nil {
		return nil, err
	}
	object := reflect.New(typ)
	dec.assignHandle(object.Interface())
	if desc.info.flags&ScExternalizable != 0 {
		return nil, errors.New("readOrdinaryObject: SC_EXTERNALIZABLE not implemented")
	}
	if err := dec.readSerialData(object, desc); err != nil {
		return nil, err
	}
	return object.Interface(), nil
}

func (dec *Decoder) readSerialData(value reflect.Value, desc *classDesc) error {
	if desc.info.superClassDesc != nil {
		superVal := reflect.ValueOf(super(value.Interface()))
		if err := dec.readSerialData(superVal, desc.info.superClassDesc); err != nil {
			return err
		}
	}
	if desc.info.flags&ScWriteMethod != 0 {
		return errors.New("readSerialData: SC_WRITE_METHOD not implemented")
	}
	dataMap := make(map[string]interface{})
	for _, field := range desc.info.fields {
		var v interface{}
		switch field.typeCode {
		default:
			fieldTyp, err := dec.typFromFieldDescriptor(string(field.typeCode))
			if err != nil {
				return err
			}
			fieldData := reflect.New(fieldTyp).Interface()
			if err := dec.readBinary(fieldData); err != nil {
				return err
			}
			v = reflect.Indirect(reflect.ValueOf(fieldData)).Interface()
		case '[':
			tc, err := dec.r.ReadByte()
			if err != nil {
				return err
			}
			if tc != TcArray {
				return fmt.Errorf("readSerialData: expected TC_ARRAY, got %02X", tc)
			}
			v, err = dec.readArray()
			if err != nil {
				return err
			}
		case 'L':
			var err error
			v, err = dec.readObject()
			if err != nil {
				return err
			}
		}
		dataMap[field.name] = v
	}
	value = reflect.Indirect(value)
	if value.Kind() != reflect.Struct {
		return fmt.Errorf("readSerialData: value should be a struct, got '%s'", value.Kind())
	}
	numField := value.NumField()
	t := value.Type()
	for i := 0; i < numField; i++ {
		tf := t.Field(i)
		// Skip unexported fields.
		if tf.PkgPath != "" {
			continue
		}
		fieldName := tf.Tag.Get("javaio")
		if fieldName == "-" {
			continue
		}
		if fieldName == "" {
			fieldName = lowerCamelCase(tf.Name)
		}
		fieldData, ok := dataMap[fieldName]
		if !ok {
			continue
		}
		fieldDataValue := reflect.ValueOf(fieldData)
		if fieldDataValue.IsValid() {
			f := value.Field(i)
			if !fieldDataValue.Type().AssignableTo(f.Type()) {
				return fmt.Errorf("readSerialData: %s is not assignable to %s", fieldDataValue.Type(), f.Type())
			}
			f.Set(fieldDataValue)
		}
	}
	return nil
}

func (dec *Decoder) typFromFieldDescriptor(fieldDesc string) (reflect.Type, error) {
	if len(fieldDesc) == 0 {
		return nil, errors.New("typFromFieldDescriptor: field descriptor should not be empty")
	}
	switch fieldDesc[0] {
	case 'B':
		return reflect.TypeOf(byte(0)), nil
	case 'D':
		return reflect.TypeOf(float64(0)), nil
	case 'F':
		return reflect.TypeOf(float32(0)), nil
	case 'I':
		return reflect.TypeOf(int32(0)), nil
	case 'J':
		return reflect.TypeOf(int64(0)), nil
	case 'S':
		return reflect.TypeOf(int16(0)), nil
	case 'Z':
		return reflect.TypeOf(false), nil
	case 'L':
		if ch := fieldDesc[len(fieldDesc)-1]; ch != ';' {
			return nil, fmt.Errorf("typFromFieldDescriptor: expected ';', got '%c'", ch)
		}
		className := strings.ReplaceAll(fieldDesc[1:len(fieldDesc)-1], "/", ".")
		return dec.getTypeFromClassName(className)
	case '[':
		elemTyp, err := dec.typFromFieldDescriptor(fieldDesc[1:])
		if err != nil {
			return nil, err
		}
		return reflect.SliceOf(elemTyp), nil
	default:
		return nil, fmt.Errorf("typFromFieldDescriptor: invalid field descriptor: %s", fieldDesc)
	}
}

func (dec *Decoder) getTypeFromClassName(className string) (reflect.Type, error) {
	typ, ok := dec.typs[className]
	if !ok {
		return nil, fmt.Errorf("typFromFieldDescriptor: class '%s' not registered", className)
	}
	return typ, nil
}
