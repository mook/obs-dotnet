package utils

// Ptr converts a value to a pointer to that value.
func Ptr[T any](input T) *T {
	return &input
}
