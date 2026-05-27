package ncmdump

import (
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
	// 1. Try to unmarshal as the classic array of [name, id]
	var v []interface{}
	if err := json.Unmarshal(data, &v); err == nil && len(v) >= 2 {
		if nameStr, ok := v[0].(string); ok {
			a.Name = nameStr
		} else {
			a.Name = fmt.Sprintf("%v", v[0])
		}
		a.Id = v[1]
		return nil
	}

	// 2. Try to unmarshal as a single string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		a.Name = s
		a.Id = nil
		return nil
	}

	// 3. Try to unmarshal as an object {"name": "...", "id": ...}
	var obj struct {
		Name string `json:"name"`
		Id   any    `json:"id"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		a.Name = obj.Name
		a.Id = obj.Id
		return nil
	}

	return fmt.Errorf("failed to unmarshal Artist from %s", string(data))
}

// @ref https://music.163.com/#/song?id={id}
type Meta struct {
	Id       any    `json:"musicId"`
	Name     string `json:"musicName"`
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
		Artists any `json:"artist"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.Artists == nil {
		return nil
	}

	// Unmarshal Artists robustly based on actual type
	if list, ok := aux.Artists.([]interface{}); ok {
		m.Artists = make([]Artist, 0, len(list))
		for _, item := range list {
			itemBytes, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var art Artist
			if err := json.Unmarshal(itemBytes, &art); err == nil {
				m.Artists = append(m.Artists, art)
			}
		}
	} else if str, ok := aux.Artists.(string); ok {
		m.Artists = []Artist{{Name: str, Id: nil}}
	} else if obj, ok := aux.Artists.(map[string]interface{}); ok {
		name, _ := obj["name"].(string)
		id := obj["id"]
		m.Artists = []Artist{{Name: name, Id: id}}
	}

	return nil
}
