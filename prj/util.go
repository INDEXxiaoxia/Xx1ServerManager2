package main

import (
	"strconv"
	"strings"
)

func itoa(v int) string  { return strconv.Itoa(v) }
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
func fieldsN(s string) []string { return strings.Fields(s) }
func joinFields(f []string) string { return strings.Join(f, " ") }
func safeF64(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func safeInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
func safeInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}