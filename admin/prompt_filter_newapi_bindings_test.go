package admin

import (
	"strconv"
)

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
