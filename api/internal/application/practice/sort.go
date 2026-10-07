package practice

import "sort"

// sortTopErrors sorts top errors descending by count, breaking ties alphabetically.
func sortTopErrors(out []TopErrorCount) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Word < out[j].Word
		}
		return out[i].Count > out[j].Count
	})
}
