package perf

import (
	"bytes"
	"log"
	"mmin/internal/encoder"
	"strconv"
)

var Randomer = encoder.NewRandomer()

type ParamsConf struct {
	Name string            `yaml:"Name"`
	Type string            `yaml:"Type"`
	Spec map[string]string `yaml:"Spec"`
}

func (pc *ParamsConf) GetParams() Params {
	var err error
	if pc.Type == "RandomInt" {
		params := &RandomInt{}
		params.start, err = strconv.Atoi(pc.Spec["Start"])
		if err != nil {
			params.start = 0
		}
		params.end, err = strconv.Atoi(pc.Spec["End"])
		if err != nil {
			params.end = 10
		}
		params.name = []byte("${" + pc.Name + "}")
		return params
	} else if pc.Type == "RandomStr" {
		params := &RandomStr{}
		params.length, err = strconv.Atoi(pc.Spec["Length"])
		if err != nil {
			params.length = 10
		}
		params.name = []byte("${" + pc.Name + "}")
		return params
	} else {
		log.Fatalf("have no %s param type", pc.Type)
	}
	return nil
}

func GetParamsMap(paramsConfs []*ParamsConf) map[string]Params {
	paramsMap := map[string]Params{}
	for _, pc := range paramsConfs {
		paramsMap[pc.Name] = pc.GetParams()
	}
	return paramsMap
}

type Params interface {
	replace(src []byte) []byte
}

type RandomInt struct {
	start int `yaml:"Start"`
	end   int `yaml:"End"`
	name  []byte
}

func (r *RandomInt) replace(src []byte) []byte {
	new := Randomer.IntBytes(r.start, r.end)
	return bytes.Replace(src, r.name, new, -1)
}

type RandomStr struct {
	length int `yaml:"Length"`
	name   []byte
}

func (r *RandomStr) replace(src []byte) []byte {
	new := Randomer.StrBytes(r.length)
	return bytes.Replace(src, r.name, new, -1)
}
