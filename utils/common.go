package utils

func GetPtrData[T any](ptr *T) (data T) {
	if ptr != nil {
		return *ptr
	}
	return
}

func MapToSlice[T any](m map[string]T) []T {
	arr := make([]T, 0, len(m))
	for _, t := range m {
		arr = append(arr, t)
	}
	return arr
}
