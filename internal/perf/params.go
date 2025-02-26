package perf

import (
	"bytes"
	"fmt"
	"mmin/internal/encoder"
	"strconv"
)

// 支持的参数类型
const (
	TypeRandomInt = "RandomInt"
	TypeRandomStr = "RandomStr"
)

var Randomer = encoder.NewRandomer()

type ParamsConf struct {
	Name string            `yaml:"Name"`
	Type string            `yaml:"Type"`
	Spec map[string]string `yaml:"Spec"`
}

func (pc *ParamsConf) GetParams() (Params, error) {
	switch pc.Type {
	case TypeRandomInt:
		return newRandomInt(pc)
	case TypeRandomStr:
		return newRandomStr(pc)
	default:
		return nil, fmt.Errorf("unsupported param type: %s", pc.Type)
	}
}

// GetParamsMap 将参数配置转换为参数映射
func GetParamsMap(paramsConfs []*ParamsConf) map[string]Params {
	paramsMap := make(map[string]Params, len(paramsConfs))
	for _, pc := range paramsConfs {
		params, err := pc.GetParams()
		if err != nil {
			continue // 跳过错误的配置
		}
		paramsMap[pc.Name] = params
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

func newRandomInt(pc *ParamsConf) (*RandomInt, error) {
	start, err := strconv.Atoi(pc.Spec["Start"])
	if err != nil {
		start = 0 // 使用默认值
	}

	end, err := strconv.Atoi(pc.Spec["End"])
	if err != nil {
		end = 10 // 使用默认值
	}

	return &RandomInt{
		start: start,
		end:   end,
		name:  []byte("${" + pc.Name + "}"),
	}, nil
}

func (r *RandomInt) replace(src []byte) []byte {
	return bytes.Replace(src, r.name, Randomer.IntBytes(r.start, r.end), -1)
}

type RandomStr struct {
	length int `yaml:"Length"`
	name   []byte
}

func newRandomStr(pc *ParamsConf) (*RandomStr, error) {
	length, err := strconv.Atoi(pc.Spec["Length"])
	if err != nil {
		length = 10 // 使用默认值
	}

	return &RandomStr{
		length: length,
		name:   []byte("${" + pc.Name + "}"),
	}, nil
}

func (r *RandomStr) replace(src []byte) []byte {
	return bytes.Replace(src, r.name, Randomer.StrBytes(r.length), -1)
}
