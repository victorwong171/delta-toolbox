package ncmdump

import (
	"os"
	"testing"
)

// BenchmarkDump benchmarks the Dump function
func BenchmarkDump(b *testing.B) {
	// 使用一个实际的NCM文件进行测试，如果没有则跳过
	fp, err := os.Open("test.ncm")
	if err != nil {
		b.Skip("No test.ncm file found, skipping benchmark")
	}
	defer fp.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次循环重新定位到文件开头
		fp.Seek(0, 0)
		Dump(fp)
	}
}

// BenchmarkBuildKeyBox benchmarks the buildKeyBox function
func BenchmarkBuildKeyBox(b *testing.B) {
	key := []byte("testkey1234567890")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildKeyBox(key)
	}
}

// BenchmarkDumpMeta benchmarks the DumpMeta function
func BenchmarkDumpMeta(b *testing.B) {
	// 使用一个实际的NCM文件进行测试，如果没有则跳过
	fp, err := os.Open("test.ncm")
	if err != nil {
		b.Skip("No test.ncm file found, skipping benchmark")
	}
	defer fp.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次循环重新定位到文件开头
		fp.Seek(0, 0)
		DumpMeta(fp)
	}
}

// BenchmarkXOR benchmarks the XOR operation in Dump function
func BenchmarkXOR(b *testing.B) {
	// 创建一个模拟的1MB数据块
	data := make([]byte, 1024*1024)
	box := buildKeyBox([]byte("testkey1234567890"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(data); j++ {
			k := byte((j + 1) & 0xff)
			data[j] ^= box[(box[k]+box[(box[k]+k)&0xff])&0xff]
		}
	}
}
