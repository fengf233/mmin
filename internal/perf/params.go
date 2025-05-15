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
	Name string   `yaml:"Name" json:"Name"`
	Type string   `yaml:"Type" json:"Type"`
	Spec []string `yaml:"Spec" json:"Spec"`
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
	start := 0 // 默认值
	end := 10  // 默认值

	if len(pc.Spec) > 0 {
		if s, err := strconv.Atoi(pc.Spec[0]); err == nil {
			start = s
		}
	}

	if len(pc.Spec) > 1 {
		if e, err := strconv.Atoi(pc.Spec[1]); err == nil {
			end = e
		}
	}

	return &RandomInt{
		start: start,
		end:   end,
		name:  []byte("${" + pc.Name + "}"),
	}, nil
}

func (r *RandomInt) replace(src []byte) []byte {
	return bytes.ReplaceAll(src, r.name, Randomer.IntBytes(r.start, r.end))
}

type RandomStr struct {
	length int `yaml:"Length"`
	name   []byte
}

func newRandomStr(pc *ParamsConf) (*RandomStr, error) {
	length := 10 // 默认值

	if len(pc.Spec) > 0 {
		if l, err := strconv.Atoi(pc.Spec[0]); err == nil {
			length = l
		}
	}

	return &RandomStr{
		length: length,
		name:   []byte("${" + pc.Name + "}"),
	}, nil
}

func (r *RandomStr) replace(src []byte) []byte {
	return bytes.ReplaceAll(src, r.name, Randomer.StrBytes(r.length))
}

// validate 验证参数配置
func (p *ParamsConf) validate() error {
	if p.Name == "" {
		return fmt.Errorf("参数名不能为空")
	}
	if p.Type == "" {
		return fmt.Errorf("参数类型不能为空")
	}
	if len(p.Spec) == 0 {
		return fmt.Errorf("参数规格不能为空")
	}
	return nil
}
