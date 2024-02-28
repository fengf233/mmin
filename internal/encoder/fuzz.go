package encoder

import (
	"math/rand"
	"time"
)

type Fuzzer struct {
	r *rand.Rand
}

func NewFuzzer() *Fuzzer {
	m := &Fuzzer{}
	m.r = rand.New(rand.NewSource(time.Now().UnixNano()))
	return m
}

func (m *Fuzzer) SetSeed(seed int64) {
	m.r.Seed(seed)
}

func (m *Fuzzer) Insert(s []byte) (r []byte) {
	pos := m.r.Intn(len(s))
	random_char := byte(m.r.Intn(95) + 32)
	r = append(r, s[:pos]...)
	r = append(r, append([]byte{random_char}, s[pos:]...)...)
	return r
}

func (m *Fuzzer) Copy(s []byte) (r []byte) {
	pos := m.r.Intn(len(s))
	copy_char := s[pos]
	r = append(r, s[:pos]...)
	r = append(r, append([]byte{copy_char}, s[pos:]...)...)
	return r
}

func (m *Fuzzer) Delete(s []byte) (r []byte) {
	if len(s) == 0 {
		return m.Insert(s)
	}
	pos := m.r.Intn(len(s))
	r = append(r, s[:pos]...)
	r = append(r, s[pos+1:]...)
	return r
}

func (m *Fuzzer) Flip(s []byte) (r []byte) {
	if len(s) == 0 {
		return m.Insert(s)
	}
	pos := m.r.Intn(len(s))
	c := byte(int(s[pos]) ^ (1 << m.r.Intn(7)))
	r = append(r, s[:pos]...)
	r = append(r, append([]byte{c}, s[pos+1:]...)...)
	return r
}

func (m *Fuzzer) Mutate(s []byte) []byte {
	Fuzzer := m.r.Intn(3)
	switch Fuzzer {
	case 0:
		return m.Insert(s)
	case 1:
		return m.Copy(s)
	case 2:
		return m.Delete(s)
	case 3:
		return m.Flip(s)
	default:
		return m.Insert(s)
	}
}

func (m *Fuzzer) Fuzz(s []byte) []byte {
	loop := m.r.Intn(len(s))
	var r []byte
	r = s
	for i := 0; i < loop; i++ {
		r = m.Mutate(r)
	}
	return r
}
