package code_test

import (
	"sort"
	"strings"
)

func joinSorted(xs []string) string {
	cp := append([]string(nil), xs...)
	sort.Strings(cp)
	return strings.Join(cp, "\n")
}
