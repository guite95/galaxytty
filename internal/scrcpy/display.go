package scrcpy

import (
	"fmt"
	"regexp"
	"strconv"
)

var displayIDRE = regexp.MustCompile(`New display:.*\(id=([0-9]+)\)`)

func ParseDisplayID(log string) (int64, error) {
	m := displayIDRE.FindStringSubmatch(log)
	if m == nil {
		return 0, fmt.Errorf("display ID not found")
	}
	return strconv.ParseInt(m[1], 10, 64)
}
