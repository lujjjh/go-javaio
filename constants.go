package javaio

// The following symbols in `java.io.ObjectStreamConstants` define
// the terminal and constant values expected in a stream.
const (
	StreamMagic      uint16 = 0xaced
	StreamVersion    int16 = 5
	TcNull           byte  = 0x70
	TcReference      byte  = 0x71
	TcClassdesc      byte  = 0x72
	TcObject         byte  = 0x73
	TcString         byte  = 0x74
	TcArray          byte  = 0x75
	TcClass          byte  = 0x76
	TcBlockdata      byte  = 0x77
	TcEndblockdata   byte  = 0x78
	TcReset          byte  = 0x79
	TcBlockdatalong  byte  = 0x7A
	TcException      byte  = 0x7B
	TcLongstring     byte  = 0x7C
	TcProxyclassdesc byte  = 0x7D
	TcEnum           byte  = 0x7E
	baseWireHandle   int32 = 0x7E0000
)

// The flag byte classDescFlags may include values of
const (
	ScWriteMethod    byte = 0x01 // if SC_SERIALIZABLE
	ScBlockData      byte = 0x08 // if SC_EXTERNALIZABLE
	ScSerializable   byte = 0x02
	ScExternalizable byte = 0x04
	ScEnum           byte = 0x10
)
