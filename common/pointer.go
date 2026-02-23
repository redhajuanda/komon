package common

import "reflect"

// ToPointer converts a value T to a pointer.
func ToPointer[T any](v T) *T {
	return &v
}

// ToPointerUnsafe is a helper function to convert any value to pointer.
// This function is unsafe because it will return nil if the value is empty/zero value.
func ToPointerUnsafe[T any](v T) *T {
	if reflect.ValueOf(v).IsZero() {
		return nil
	}
	return &v
}

// ToPointerUnsafeInterface is a helper function to convert any value to pointer.
// This function is unsafe because it will return nil if the value is empty/zero value.
// This function is used when the value is an interface, but the type is known.
func ToPointerUnsafeInterface[T any](v any) *T {
	if v == nil {
		return nil
	}

	// Use reflection to handle zero value checks
	val, ok := v.(T)
	if !ok {
		return nil
	}

	// Create a zero value of type T using reflection
	if reflect.ValueOf(val).IsZero() {
		return nil
	}

	return &val
}

// FromPointer converts a pointer to a value T.
func FromPointer[T any](v *T) T {
	if v == nil {
		// Create a zero value of type T
		var zero T
		return zero
	}
	return *v
}

func DereferencePointer(val any) any {
	// Check if the value is a pointer
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		return v.Elem().Interface()
	}
	return val
}
