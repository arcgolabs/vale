// Package genericx contains small helpers for generic interface boundaries.
package genericx

import "reflect"

// IsNil reports whether value is nil, including a typed nil stored behind an
// interface. Reflection is isolated here because Go has no type-safe operation
// that detects every nil-capable type after generic values are interface-erased.
func IsNil[T any](value T) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	case reflect.Invalid:
		return true
	case reflect.UnsafePointer:
		return reflected.IsZero()
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128,
		reflect.Array, reflect.String, reflect.Struct:
		return false
	}
	return false
}
