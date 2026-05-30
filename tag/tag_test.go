package tag

import (
	"os"
	"testing"
)

func TestNewFlacTagger(t *testing.T) {
	path := "test.flac"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("skipping test: %s not found", path)
	}

	tagger, err := NewFlacTagger(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = tagger.SetComment("11111")
	_ = tagger.SetTitle("66666")
	err = tagger.Save()
	t.Log(err)
}
