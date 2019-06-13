package javaio

import (
	"encoding/binary"
	"errors"
	"io"
)

type Decoder struct {
	r io.Reader
}

func NewDecoder(r io.Reader) (*Decoder, error) {
	dec := &Decoder{
		r: r,
	}
	if err := dec.readHeader(); err != nil {
		return nil, err
	}
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

func (dec *Decoder) readObject(object interface{}) error {
	return nil
}
