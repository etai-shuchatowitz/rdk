// Code generated by the FlatBuffers compiler. DO NOT EDIT.

package tflite

import (
	flatbuffers "github.com/google/flatbuffers/go"
)

type SequenceRNNOptions struct {
	_tab flatbuffers.Table
}

func GetRootAsSequenceRNNOptions(buf []byte, offset flatbuffers.UOffsetT) *SequenceRNNOptions {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &SequenceRNNOptions{}
	x.Init(buf, n+offset)
	return x
}

func GetSizePrefixedRootAsSequenceRNNOptions(buf []byte, offset flatbuffers.UOffsetT) *SequenceRNNOptions {
	n := flatbuffers.GetUOffsetT(buf[offset+flatbuffers.SizeUint32:])
	x := &SequenceRNNOptions{}
	x.Init(buf, n+offset+flatbuffers.SizeUint32)
	return x
}

func (rcv *SequenceRNNOptions) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *SequenceRNNOptions) Table() flatbuffers.Table {
	return rcv._tab
}

func (rcv *SequenceRNNOptions) TimeMajor() bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.GetBool(o + rcv._tab.Pos)
	}
	return false
}

func (rcv *SequenceRNNOptions) MutateTimeMajor(n bool) bool {
	return rcv._tab.MutateBoolSlot(4, n)
}

func (rcv *SequenceRNNOptions) FusedActivationFunction() ActivationFunctionType {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return ActivationFunctionType(rcv._tab.GetInt8(o + rcv._tab.Pos))
	}
	return 0
}

func (rcv *SequenceRNNOptions) MutateFusedActivationFunction(n ActivationFunctionType) bool {
	return rcv._tab.MutateInt8Slot(6, int8(n))
}

func (rcv *SequenceRNNOptions) AsymmetricQuantizeInputs() bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.GetBool(o + rcv._tab.Pos)
	}
	return false
}

func (rcv *SequenceRNNOptions) MutateAsymmetricQuantizeInputs(n bool) bool {
	return rcv._tab.MutateBoolSlot(8, n)
}

func SequenceRNNOptionsStart(builder *flatbuffers.Builder) {
	builder.StartObject(3)
}
func SequenceRNNOptionsAddTimeMajor(builder *flatbuffers.Builder, timeMajor bool) {
	builder.PrependBoolSlot(0, timeMajor, false)
}
func SequenceRNNOptionsAddFusedActivationFunction(builder *flatbuffers.Builder, fusedActivationFunction ActivationFunctionType) {
	builder.PrependInt8Slot(1, int8(fusedActivationFunction), 0)
}
func SequenceRNNOptionsAddAsymmetricQuantizeInputs(builder *flatbuffers.Builder, asymmetricQuantizeInputs bool) {
	builder.PrependBoolSlot(2, asymmetricQuantizeInputs, false)
}
func SequenceRNNOptionsEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
