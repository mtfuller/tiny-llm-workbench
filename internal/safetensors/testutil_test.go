package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

// testTensor is one tensor to write into a fake safetensors file via
// writeTestSafetensors.
type testTensor struct {
	Dtype string
	Shape []int64
	Data  []byte
}

// f32Bytes packs vs as little-endian float32 bytes, for building fake
// tensor data in tests.
func f32Bytes(vs ...float32) []byte {
	buf := make([]byte, 4*len(vs))
	for i, v := range vs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// writeTestSafetensors writes a minimal, real safetensors file (8-byte
// header length + JSON header + concatenated tensor bytes) to path.
func writeTestSafetensors(t *testing.T, path string, tensors map[string]testTensor) {
	t.Helper()

	names := make([]string, 0, len(tensors))
	for name := range tensors {
		names = append(names, name)
	}
	sort.Strings(names)

	header := make(map[string]any, len(tensors))
	var data []byte
	for _, name := range names {
		tv := tensors[name]
		start := int64(len(data))
		data = append(data, tv.Data...)
		header[name] = map[string]any{
			"dtype":        tv.Dtype,
			"shape":        tv.Shape,
			"data_offsets": []int64{start, int64(len(data))},
		}
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal test header: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test safetensors file: %v", err)
	}
	defer f.Close()

	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(headerBytes)))
	if _, err := f.Write(lenBuf[:]); err != nil {
		t.Fatalf("write header length: %v", err)
	}
	if _, err := f.Write(headerBytes); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write tensor data: %v", err)
	}
}
