package javaio

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
)

type Decoder struct {
	r             *bufio.Reader
	handles       []interface{}
	blockDataMode bool
}

func NewDecoder(r io.Reader) (*Decoder, error) {
	dec := &Decoder{
		r: bufio.NewReader(r),
	}
	if err := dec.readHeader(); err != nil {
		return nil, err
	}
	dec.blockDataMode = true
	return dec, nil
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

func (dec *Decoder) ReadObject(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("non-pointer type")
	}
	return dec.readObject(rv)
}

func (dec *Decoder) readObject(v reflect.Value) error {
	tc, err := dec.r.ReadByte()
	if err != nil {
		return err
	}

	switch tc {
	case TcNull:
		return nil
	case TcReference:
		return dec.readHandle(v)
	}

	return nil
}

func (dec *Decoder) readHandle(v reflect.Value) error {
	var passHandle int32
	if err := dec.readBinary(&passHandle); err != nil {
		return err
	}
	passHandle -= baseWireHandle
	if passHandle < 0 || int(passHandle) > len(dec.handles) {
		return fmt.Errorf("invalid handle value: %d", passHandle+baseWireHandle)
	}
	obj := dec.handles[passHandle]
	v = reflect.Indirect(v)
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		v.Set(reflect.ValueOf(obj))
		return nil
	}
	return nil
}
