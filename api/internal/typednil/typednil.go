// Package typednil checks whether an interface value holds a nil pointer or function.
package typednil

import "reflect"

// Is reports whether v is nil or contains a typed nil (e.g. (*T)(nil)).
func Is(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Func, reflect.UnsafePointer:
		if rv.IsNil() {
			return true
		}
		if rv.Kind() == reflect.Pointer && rv.Type().Elem().Kind() == reflect.Interface {
			return rv.Elem().IsNil()
		}
		return false
	case reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

