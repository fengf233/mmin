package encoder

import (
	"math/rand"
	"strconv"
	"time"
)

type Randomer struct {
	rand           *rand.Rand
	strSeedByte    []byte
	strSeedByteLen int
}

func NewRandomer() *Randomer {
	r := &Randomer{}
	r.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	r.strSeedByte = []byte("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	r.strSeedByteLen = len(r.strSeedByte)
	return r
}

func (r *Randomer) Int(min, max int) int {
	return min + r.rand.Intn(max-min)
}

func (r *Randomer) IntStr(min, max int) string {
	return strconv.Itoa(r.Int(min, max))
}

func (r *Randomer) IntBytes(min, max int) []byte {
	return []byte(r.IntStr(min, max))
}

func (r *Randomer) Str(length int) string {
	result := []byte{}
	for i := 0; i < length; i++ {
		result = append(result, r.strSeedByte[r.rand.Intn(r.strSeedByteLen)])
	}
	return string(result)
}

func (r *Randomer) StrBytes(length int) []byte {
	result := []byte{}
	for i := 0; i < length; i++ {
		result = append(result, r.strSeedByte[r.rand.Intn(r.strSeedByteLen)])
	}
	return result
}
