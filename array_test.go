package javaio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArray_ClassName(t *testing.T) {
	assert.Equal(t, "[[I", NewArray([][]int32{}).ClassName())
}

func TestArray_SerialVersionUID(t *testing.T) {
	assert.Equal(t, int64(1727100010502261052), NewArray([][]int32{}).SerialVersionUID())
}
