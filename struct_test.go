package ncmdump

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestArtistUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		want    Artist
		wantErr bool
	}{
		{
			name:    "classic array [name, id]",
			jsonStr: `["Artist Name", 12345]`,
			want:    Artist{Name: "Artist Name", Id: float64(12345)},
		},
		{
			name:    "single string",
			jsonStr: `"Artist Name"`,
			want:    Artist{Name: "Artist Name", Id: nil},
		},
		{
			name:    "object with name and id",
			jsonStr: `{"name": "Artist Name", "id": 67890}`,
			want:    Artist{Name: "Artist Name", Id: float64(67890)},
		},
		{
			name:    "invalid format",
			jsonStr: `12345`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Artist
			err := json.Unmarshal([]byte(tt.jsonStr), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if got.Name != tt.want.Name || !reflect.DeepEqual(got.Id, tt.want.Id) {
					t.Errorf("UnmarshalJSON() = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestMetaUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		want    Meta
		wantErr bool
	}{
		{
			name: "artist is classic array of arrays",
			jsonStr: `{
				"musicId": 111,
				"musicName": "Song 1",
				"album": "Album 1",
				"albumPic": "http://pic1",
				"artist": [["Artist A", 1001], ["Artist B", 1002]],
				"format": "flac"
			}`,
			want: Meta{
				Id:     float64(111),
				Name:   "Song 1",
				Format: "flac",
				Album: &Album{
					Id:       nil,
					Name:     "Album 1",
					CoverUrl: "http://pic1",
				},
				Artists: []Artist{
					{Name: "Artist A", Id: float64(1001)},
					{Name: "Artist B", Id: float64(1002)},
				},
			},
		},
		{
			name: "artist is string",
			jsonStr: `{
				"musicId": 222,
				"musicName": "Song 2",
				"artist": "Solo Artist",
				"format": "mp3"
			}`,
			want: Meta{
				Id:      float64(222),
				Name:    "Song 2",
				Format:  "mp3",
				Artists: []Artist{{Name: "Solo Artist", Id: nil}},
			},
		},
		{
			name: "artist is object",
			jsonStr: `{
				"musicId": 333,
				"musicName": "Song 3",
				"artist": {"name": "Obj Artist", "id": 3001},
				"format": "flac"
			}`,
			want: Meta{
				Id:     float64(333),
				Name:   "Song 3",
				Format: "flac",
				Artists: []Artist{
					{Name: "Obj Artist", Id: float64(3001)},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Meta
			err := json.Unmarshal([]byte(tt.jsonStr), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if got.Name != tt.want.Name || !reflect.DeepEqual(got.Id, tt.want.Id) || got.Format != tt.want.Format {
					t.Errorf("got metadata name/id/format mismatch")
				}
				if tt.want.Album != nil {
					if got.Album == nil || got.Album.Name != tt.want.Album.Name || got.Album.CoverUrl != tt.want.Album.CoverUrl {
						t.Errorf("got album mismatch: %+v vs %+v", got.Album, tt.want.Album)
					}
				}
				if len(got.Artists) != len(tt.want.Artists) {
					t.Errorf("got artist length mismatch: len %d, want %d", len(got.Artists), len(tt.want.Artists))
				} else {
					for i := range got.Artists {
						if got.Artists[i].Name != tt.want.Artists[i].Name || !reflect.DeepEqual(got.Artists[i].Id, tt.want.Artists[i].Id) {
							t.Errorf("artist at %d mismatch: %+v vs %+v", i, got.Artists[i], tt.want.Artists[i])
						}
					}
				}
			}
		})
	}
}
