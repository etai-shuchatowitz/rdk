// Code generated by the FlatBuffers compiler. DO NOT EDIT.

package tflite

import (
	flatbuffers "github.com/google/flatbuffers/go"
)

type SelectV2Options struct {
	_tab flatbuffers.Table
}

func GetRootAsSelectV2Options(buf []byte, offset flatbuffers.UOffsetT) *SelectV2Options {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &SelectV2Options{}
	x.Init(buf, n+offset)
	return x
}

func GetSizePrefixedRootAsSelectV2Options(buf []byte, offset flatbuffers.UOffsetT) *SelectV2Options {
	n := flatbuffers.GetUOffsetT(buf[offset+flatbuffers.SizeUint32:])
	x := &SelectV2Options{}
	x.Init(buf, n+offset+flatbuffers.SizeUint32)
	return x
}

func (rcv *SelectV2Options) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *SelectV2Options) Table() flatbuffers.Table {
	return rcv._tab
}

func SelectV2OptionsStart(builder *flatbuffers.Builder) {
	builder.StartObject(0)
}
func SelectV2OptionsEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
