package tag

import "github.com/bogem/id3v2"

// Mp3Tagger 实现了 MP3 音频格式的 ID3v2 标签与封面图片注入工具
type Mp3Tagger struct {
	tag *id3v2.Tag
}

// NewMp3Tagger 打开并解析指定路径的 MP3 文件，返回包装后的 Mp3Tagger 实例
func NewMp3Tagger(path string) (*Mp3Tagger, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return nil, err
	}
	tagger := new(Mp3Tagger)
	tagger.tag = tag

	return tagger, nil
}

// SetCover 构造标准 FrontCover 类型的图片帧，并将其注入 MP3 的 ID3 标签中
func (m *Mp3Tagger) SetCover(buf []byte, mime string) error {
	m.tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingISO,
		MimeType:    mime,
		PictureType: id3v2.PTFrontCover,
		Description: "Front cover",
		Picture:     buf,
	})
	return nil
}

// SetCoverUrl 将远程封面图片 URL 以外部链接链接图片帧的形式注入到 MP3 的 ID3 标签中
func (m *Mp3Tagger) SetCoverUrl(coverUrl string) error {
	m.tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingISO,
		MimeType:    "-->",
		PictureType: id3v2.PTFrontCover,
		Description: "Front cover",
		Picture:     []byte(coverUrl),
	})
	return nil
}

// SetTitle 注入歌曲标题，若文件已含有非空标题则跳过以防覆盖
func (m *Mp3Tagger) SetTitle(title string) error {
	if name := m.tag.Title(); name == "" {
		m.tag.SetTitle(title)
	}
	return nil
}

// SetAlbum 注入专辑名，若文件已含有非空专辑名则跳过以防覆盖
func (m *Mp3Tagger) SetAlbum(album string) error {
	if name := m.tag.Album(); name == "" {
		m.tag.SetAlbum(album)
	}
	return nil
}

// SetArtist 注入多位艺术家，若文件已含有非空艺术家列表则跳过以防覆盖
func (m *Mp3Tagger) SetArtist(artists []string) error {
	if frames := m.tag.GetFrames(m.tag.CommonID("Artist")); len(frames) == 0 {
		for _, artist := range artists {
			m.tag.SetArtist(artist)
		}
	}
	return nil
}

// SetComment 注入备注注释信息到 ID3 标签帧中
func (m *Mp3Tagger) SetComment(comment string) error {
	if frames := m.tag.GetFrames(m.tag.CommonID("Comments")); len(frames) == 0 {
		m.tag.AddCommentFrame(id3v2.CommentFrame{
			Encoding:    id3v2.EncodingISO,
			Language:    "XXX",
			Description: "",
			Text:        comment,
		})
	}
	return nil
}

// Save 写盘持久化并关闭 ID3 标签句柄，必须最终调用以释放文件锁
func (m *Mp3Tagger) Save() error {
	err := m.tag.Save()
	_ = m.tag.Close()
	return err
}

