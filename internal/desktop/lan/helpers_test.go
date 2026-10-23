package lan

import (
	"encoding/pem"
	"strconv"
)

func itoa(n int) string { return strconv.Itoa(n) }

func pemDecode(b []byte) ([]byte, []byte) {
	blk, rest := pem.Decode(b)
	if blk == nil {
		return nil, rest
	}
	return blk.Bytes, rest
}
