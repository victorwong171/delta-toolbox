package ncmdump

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
)

var (
	aesCoreKey   = []byte{0x68, 0x7A, 0x48, 0x52, 0x41, 0x6D, 0x73, 0x6F, 0x35, 0x6B, 0x49, 0x6E, 0x62, 0x61, 0x78, 0x57}
	aesModifyKey = []byte{0x23, 0x31, 0x34, 0x6C, 0x6A, 0x6B, 0x5F, 0x21, 0x5C, 0x5D, 0x26, 0x30, 0x55, 0x3C, 0x27, 0x28}

	// bufferPool is used to reuse read buffers, reducing memory allocation
	bufferPool = sync.Pool{
		New: func() interface{} {
			// 32KB buffer size
			return make([]byte, 0x8000)
		},
	}
)

func buildKeyBox(key []byte) []byte {
	box := make([]byte, 256)
	for i := 0; i < 256; i++ {
		box[i] = byte(i)
	}
	keyLen := byte(len(key))
	var c, lastByte, keyOffset byte
	for i := 0; i < 256; i++ {
		c = (box[i] + lastByte + key[keyOffset]) & 0xff
		keyOffset++
		if keyOffset >= keyLen {
			keyOffset = 0
		}
		box[i], box[c] = box[c], box[i]
		lastByte = c
	}
	return box
}

// NCMFile checks if the file is a valid NCM file without changing file pointer
func NCMFile(fp *os.File) (bool, error) {
	// Save current file position
	currentPos, err := fp.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	defer fp.Seek(currentPos, io.SeekStart) // Restore position

	// Jump to begin of file
	if _, err := fp.Seek(0, io.SeekStart); err != nil {
		return false, err
	}

	var header = make([]byte, 8)
	if err := binary.Read(fp, binary.LittleEndian, &header); err != nil {
		return false, nil
	}

	if string(header) != "CTENFDAM" {
		return false, fmt.Errorf("%s isn't netease cloud music copyright file", fp.Name())
	}

	return true, nil
}

// Decode decodes the key data without changing file pointer
func Decode(fp *os.File) ([]byte, error) {
	// Save current file position
	currentPos, err := fp.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	defer fp.Seek(currentPos, io.SeekStart) // Restore position

	// jump over the magic head(4*2) and the gap(2).
	if _, err := fp.Seek(4*2+2, io.SeekStart); err != nil {
		return nil, err
	}

	keyData, err := readLenAndData(fp)
	xorBytes(keyData, 0x64)

	deKeyData, err := decryptAes128Ecb(aesCoreKey, keyData)
	if err != nil {
		return nil, err
	}

	// 17 = len("neteasecloudmusic")
	return deKeyData[17:], nil
}

const (
	FLAC = "flac"
	MP3  = "mp3"
)

func getNcmFileFormat(fp *os.File) string {
	info, err := fp.Stat()
	if err != nil {
		return MP3
	}
	if info.Size() < int64(math.Pow(1024, 2)*16) {
		return MP3
	}
	return FLAC
}

// DumpMeta extracts metadata without changing file pointer
func DumpMeta(fp *os.File) (Meta, error) {
	// Save current file position
	currentPos, err := fp.Seek(0, io.SeekCurrent)
	if err != nil {
		return Meta{}, err
	}
	defer fp.Seek(currentPos, io.SeekStart) // Restore position

	// jump over the magic head(4*2) and the gap(2).
	if _, err := fp.Seek(4*2+2, io.SeekStart); err != nil {
		return Meta{}, err
	}

	// Skip key data
	if _, err := readLenAndData(fp); err != nil {
		return Meta{}, err
	}

	modifyData, err := readLenAndData(fp)
	if err != nil {
		return Meta{}, err
	}

	if len(modifyData) == 0 {
		return Meta{
			Format: getNcmFileFormat(fp),
		}, nil
	}

	xorBytes(modifyData, 0x63)

	// 22 = len(`163 key(Don't modify):`)
	deModifyData := make([]byte, base64.StdEncoding.DecodedLen(len(modifyData)-22))
	if _, err = base64.StdEncoding.Decode(deModifyData, modifyData[22:]); err != nil {
		return Meta{}, err
	}

	deData, err := decryptAes128Ecb(aesModifyKey, deModifyData)
	if err != nil {
		return Meta{}, err
	}

	// 6 = len("music:")
	deData = deData[6:]

	var meta Meta
	if err := json.Unmarshal(deData, &meta); err != nil {
		return Meta{}, err
	}

	meta.Comment = string(modifyData)
	return meta, nil
}

// DumpCover extracts cover data without changing file pointer
func DumpCover(fp *os.File) ([]byte, error) {
	// Save current file position
	currentPos, err := fp.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	defer fp.Seek(currentPos, io.SeekStart) // Restore position

	// jump over the magic head(4*2) and the gap(2).
	if _, err := fp.Seek(4*2+2, io.SeekStart); err != nil {
		return nil, err
	}

	// Skip key data
	if _, err := readLenAndData(fp); err != nil {
		return nil, err
	}

	// Skip modify data
	if _, err := readLenAndData(fp); err != nil {
		return nil, err
	}

	// jump over crc32 check
	if _, err := fp.Seek(9, io.SeekCurrent); err != nil {
		return nil, err
	}

	return readLenAndData(fp)
}

// DumpToWriter dumps the decrypted data to a writer, reducing memory usage
func DumpToWriter(fp *os.File, w io.Writer) error {
	if result, err := NCMFile(fp); !result || err != nil {
		return err
	}

	// whether decode key is successful
	deKeyData, err := Decode(fp)
	if err != nil {
		return err
	}

	// Jump over headers to position the file pointer at the start of the actual audio data
	if _, err := fp.Seek(4*2+2, io.SeekStart); err != nil {
		return err
	}
	if _, err := readLenAndData(fp); err != nil {
		return err
	}
	if _, err := readLenAndData(fp); err != nil {
		return err
	}
	if _, err := fp.Seek(9, io.SeekCurrent); err != nil {
		return err
	}
	if _, err := readLenAndData(fp); err != nil {
		return err
	}

	box := buildKeyBox(deKeyData)

	// Precompute lookup table for XOR operation to avoid repeated calculations
	xorLookup := make([]byte, 256)
	for j := 0; j < 256; j++ {
		bj := byte(j)
		xorLookup[bj] = box[(box[bj]+box[(box[bj]+bj)&0xff])&0xff]
	}

	// Get buffer from pool, reducing memory allocation
	tb := bufferPool.Get().([]byte)
	defer bufferPool.Put(tb) // Return buffer to pool when done

	streamOffset := 0
	for {
		readSize, err := fp.Read(tb)
		if err != nil {
			if err != io.EOF {
				return err
			}
			if readSize == 0 {
				break
			}
		}

		// Process the bytes using precomputed lookup table and global stream offset
		for i := 0; i < readSize; i++ {
			j := byte((streamOffset + i + 1) & 0xff)
			tb[i] ^= xorLookup[j]
		}

		if _, err := w.Write(tb[:readSize]); err != nil {
			return err
		}

		streamOffset += readSize
	}

	return nil
}

// Dump dumps the decrypted data to memory (backward compatibility)
func Dump(fp *os.File) ([]byte, error) {
	var writer bytes.Buffer
	err := DumpToWriter(fp, &writer)
	return writer.Bytes(), err
}
