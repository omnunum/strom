package main

import "fmt"

//
// Returns unique items in a slice based on
// their .String() representation.
//
func Unique[S fmt.Stringer](slice []S) []S {
	// create a map with all the values as key
	uniqMap := make(map[string]S)
	for _, v := range slice {
		uniqMap[v.String()] = v
	}

	// turn the map keys into a slice
	uniqSlice := make([]S, 0, len(uniqMap))
	for v := range uniqMap {
		uniqSlice = append(uniqSlice, uniqMap[v])
	}
	return uniqSlice
}
