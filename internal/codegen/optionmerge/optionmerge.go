// Package optionmerge provides shared merge logic for Platform's AuthN and
// AuthZ protoc plugins. Both plugins use identical service-default and
// method-override semantics:
//
//   - a method-level rule with mode != 0 fully replaces the service default,
//   - an absent method rule or mode == 0 inherits the service default,
//   - if neither side contributes a non-zero mode, no rule applies.
//
// Mode is conventionally field number 1 in each rule proto message.
package optionmerge

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const modeFieldNumber protoreflect.FieldNumber = 1

// Merge returns the effective rule after applying service-level and
// method-level merge semantics. The result is always a deep clone.
func Merge[T proto.Message](svcDefault, methodRule T, hasMethod bool) (T, bool) {
	var zero T

	if hasMethod && !isNil(methodRule) && modeNonZero(methodRule) {
		return proto.Clone(methodRule).(T), true
	}
	if !isNil(svcDefault) && modeNonZero(svcDefault) {
		return proto.Clone(svcDefault).(T), true
	}
	return zero, false
}

func modeNonZero(m proto.Message) bool {
	md := m.ProtoReflect().Descriptor()
	fd := md.Fields().ByNumber(modeFieldNumber)
	if fd == nil {
		return false
	}
	return m.ProtoReflect().Get(fd).Enum() != 0
}

func isNil[T proto.Message](m T) bool {
	return proto.Message(m) == nil || !m.ProtoReflect().IsValid()
}
