package machine

import (
	"fmt"
	"reflect"
)

// NodeCompare compares all nodes. The ID can be different, but all sub-values must be the same.
func NodeCompare(lhs, rhs *NodeIL) bool {
	return nCompare(lhs, rhs, 0)
}

func nCompare(lhs, rhs *NodeIL, d int) bool {
	if lhs == nil && rhs == nil {
		return false // _technically_ they are the same, but this probably isn't want you wanted.
	}
	if lhs == nil || rhs == nil {
		return false
	}
	if lhs.Kind != rhs.Kind {
		return false
	}
	if lhs.SubType != rhs.SubType {
		return false
	}
	if len(lhs.Children) != len(rhs.Children) {
		return false
	}
	if !nCompareV(lhs.Value, rhs.Value) {
		return false
	}
	if !(lhs.Chained == nil && rhs.Chained == nil) && !nCompare(lhs.Chained, rhs.Chained, d+1) {
		return false
	}

	for i := 0; i < len(lhs.Children); i++ {
		lc := lhs.Children[i]
		rc := rhs.Children[i]
		if !nCompare(lc, rc, d+1) {
			return false
		}
	}

	return true
}

func nCompareV(lhs, rhs *NodeIL_DValue) bool {
	if lhs == nil && rhs == nil {
		return true
	}
	if lhs == nil || rhs == nil {
		return false
	}
	if lhs.Kind != rhs.Kind {
		return false
	}

	return lhs.value().Interface() == rhs.value().Interface()
}

func (n *NodeIL_DValue) value() reflect.Value {
	switch n.Kind {
	case NodeIL_DValue_STR:
		return reflect.ValueOf(n.Str)
	case NodeIL_DValue_FLT:
		return reflect.ValueOf(n.Flt)
	case NodeIL_DValue_BOOL:
		return reflect.ValueOf(n.Bool)
	default:
		panic(fmt.Errorf("unknown node value kind: %s", n.Kind))
	}
}
