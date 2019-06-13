package javaio

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDecoder(t *testing.T) {
	_, err := NewDecoder(bytes.NewReader([]byte{0xac, 0xed, 0x00, 0x05}))
	assert.NoError(t, err)

	_, err = NewDecoder(bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x05}))
	assert.Error(t, err)

	_, err = NewDecoder(bytes.NewReader([]byte{0xac, 0xed, 0x00, 0x00}))
	assert.Error(t, err)
}
