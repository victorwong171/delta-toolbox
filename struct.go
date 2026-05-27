package ncmdump

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type Album struct {
	Id       any    `json:"albumId"`
	Name     string `json:"album"`
	CoverUrl string `json:"albumPic"`
}

type Artist struct {
	Name string
	Id   any
}

// UnmarshalJSON handles various artist metadata formats (array of arrays, objects, strings) robustly
func (a *Artist) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty json data")
	}

	switch trimmed[0] {
	case '[':
		// 1. Try to unmarshal as the classic array of [name, id]
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
		// 2. Try to unmarshal as a single string
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			a.Name = s
			a.Id = nil
			return nil
		}
	case '{':
		// 3. Try to unmarshal as an object {"name": "...", "id": ...}
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

// @ref https://music.163.com/#/song?id={id}
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

// UnmarshalJSON handles various artist formats (array of arrays, array of objects, strings, objects) robustly
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

	// Unmarshal Artists robustly based on actual type
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
