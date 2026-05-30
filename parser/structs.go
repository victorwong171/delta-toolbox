package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Album 代表专辑元数据信息
type Album struct {
	Id       any    `json:"albumId"`
	Name     string `json:"album"`
	CoverUrl string `json:"albumPic"`
}

// Artist 代表艺术家/歌手元数据信息
type Artist struct {
	Name string
	Id   any
}

// UnmarshalJSON 实现了对 Artist 结构的鲁棒反序列化，能够兼容处理各种格式的歌手元数据（如经典的 [name, id] 数组、纯对象或单字符串形式）
func (a *Artist) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty json data")
	}

	switch trimmed[0] {
	case '[':
		// 1. 尝试解析为经典的数组形式 [name, id]
		var v []interface{}
		if err := json.Unmarshal(trimmed, &v); err == nil && len(v) >= 2 {
			if nameStr, ok := v[0].(string); ok {
				a.Name = nameStr
			} else {
				a.Name = fmt.Sprintf("%v", v[0])
			}
			a.Id = v[1]
			return nil
		}
	case '"':
		// 2. 尝试解析为单一字符串形式
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			a.Name = s
			a.Id = nil
			return nil
		}
	case '{':
		// 3. 尝试解析为标准对象结构 {"name": "...", "id": ...}
		var obj struct {
			Name string `json:"name"`
			Id   any    `json:"id"`
		}
		if err := json.Unmarshal(trimmed, &obj); err == nil {
			a.Name = obj.Name
			a.Id = obj.Id
			return nil
		}
	}

	return fmt.Errorf("failed to unmarshal Artist from %s", string(trimmed))
}

// Meta 代表音频轨道的全局元数据信息（包括歌曲ID、歌曲名、专辑、歌手、码率、时长与格式）
type Meta struct {
	Id       any      `json:"musicId"`
	Name     string   `json:"musicName"`
	*Album   `json:",inline"`
	Artists  []Artist `json:"artist"`
	BitRate  any      `json:"bitrate"`
	Duration any      `json:"duration"`
	Format   string   `json:"format"`
	Comment  string   `json:"-"`
}

// UnmarshalJSON 实现了对 Meta 结构的鲁棒反序列化，能够兼容不同层级和形态的歌手嵌套元数据（如数组嵌套、纯对象嵌套或简单字符串）
func (m *Meta) UnmarshalJSON(data []byte) error {
	type Alias Meta
	aux := &struct {
		Artists json.RawMessage `json:"artist"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Artists) == 0 || string(aux.Artists) == "null" {
		return nil
	}

	// 依据实际数据类型，鲁棒地反序列化歌手列表
	var list []Artist
	if err := json.Unmarshal(aux.Artists, &list); err == nil {
		m.Artists = list
		return nil
	}

	var art Artist
	if err := json.Unmarshal(aux.Artists, &art); err == nil {
		m.Artists = []Artist{art}
		return nil
	}

	var str string
	if err := json.Unmarshal(aux.Artists, &str); err == nil {
		m.Artists = []Artist{{Name: str, Id: nil}}
		return nil
	}

	return nil
}

