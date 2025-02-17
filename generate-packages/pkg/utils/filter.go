package utils

import (
	"iter"
	"slices"
)

func FilterIter[V any](seq iter.Seq[V], callback func(V) bool) iter.Seq[V] {
	return func(yield func(V) bool) {
		for v := range seq {
			if callback(v) {
				if !yield(v) {
					return
				}
			}
		}
	}
}

func Filter[V any](S []V, callback func(V) bool) []V {
	return slices.Collect(FilterIter(slices.Values(S), callback))
}
