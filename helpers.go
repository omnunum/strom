package main

//
// Returns unique items in a slice
//
func Unique(s []any) []any {
	// create a map with all the values as keys
	seen := make(map[any]bool)
	unique := make([]any, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}
	return unique
}
