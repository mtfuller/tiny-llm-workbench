// Package safetensors reads just enough of a .safetensors model directory —
// header metadata and, on demand, a single tensor's raw bytes — to power
// the Models page's architecture and weight-heatmap visualizations, without
// ever loading a whole (potentially multi-gigabyte) model into memory.
//
// This deliberately parses files directly on the local filesystem rather
// than the technique the tool's design doc suggested (the browser's
// File.slice() API against a user-picked file): TLW already knows a
// registry model's on-disk path server-side (registry.Model.Path), so
// there's nothing to gain from asking the user to re-locate a file the app
// already has — and reading exact byte ranges via os.File.ReadAt achieves
// the same "don't load the whole file" goal from the backend instead.
package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// TensorInfo locates one tensor's raw bytes within a model directory's
// safetensors file(s).
type TensorInfo struct {
	Name  string
	Dtype string
	Shape []int64

	// File is the absolute path to the safetensors file holding this
	// tensor's bytes (a model directory can shard tensors across several).
	File string
	// DataStart/DataEnd are absolute byte offsets within File — already
	// adjusted for that file's own 8-byte length prefix and JSON header, so
	// callers can pass them straight to os.File.ReadAt.
	DataStart int64
	DataEnd   int64
}

// NumElements is the product of Shape's dimensions.
func (t TensorInfo) NumElements() int64 {
	n := int64(1)
	for _, d := range t.Shape {
		n *= d
	}
	return n
}

// rawTensorMeta mirrors one non-metadata entry in a safetensors file's JSON
// header.
type rawTensorMeta struct {
	Dtype       string   `json:"dtype"`
	Shape       []int64  `json:"shape"`
	DataOffsets [2]int64 `json:"data_offsets"`
}

// indexFile mirrors a sharded model's model.safetensors.index.json.
type indexFile struct {
	WeightMap map[string]string `json:"weight_map"`
}

// ParseModelDir reads the safetensors header(s) in dir (a registry model's
// directory) and returns every tensor's location, without reading any
// tensor's actual weight bytes. Handles both a single model.safetensors
// file and a sharded model.safetensors.index.json + multiple shard files.
func ParseModelDir(dir string) ([]TensorInfo, error) {
	if data, err := os.ReadFile(filepath.Join(dir, "model.safetensors.index.json")); err == nil {
		return parseSharded(dir, data)
	}

	if singlePath := filepath.Join(dir, "model.safetensors"); fileExists(singlePath) {
		return parseSingleFile(singlePath)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "*.safetensors"))
	if len(matches) > 0 {
		return parseSingleFile(matches[0])
	}

	return nil, fmt.Errorf("no .safetensors files found in %s", dir)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseSingleFile(path string) ([]TensorInfo, error) {
	raw, dataBase, err := parseFileHeader(path)
	if err != nil {
		return nil, err
	}

	tensors := make([]TensorInfo, 0, len(raw))
	for name, meta := range raw {
		tensors = append(tensors, TensorInfo{
			Name:      name,
			Dtype:     meta.Dtype,
			Shape:     meta.Shape,
			File:      path,
			DataStart: dataBase + meta.DataOffsets[0],
			DataEnd:   dataBase + meta.DataOffsets[1],
		})
	}
	sort.Slice(tensors, func(i, j int) bool { return tensors[i].Name < tensors[j].Name })
	return tensors, nil
}

func parseSharded(dir string, indexData []byte) ([]TensorInfo, error) {
	var idx indexFile
	if err := json.Unmarshal(indexData, &idx); err != nil {
		return nil, fmt.Errorf("parse safetensors index: %w", err)
	}

	namesByFile := make(map[string][]string)
	for name, file := range idx.WeightMap {
		namesByFile[file] = append(namesByFile[file], name)
	}

	var tensors []TensorInfo
	for file, names := range namesByFile {
		path := filepath.Join(dir, file)
		raw, dataBase, err := parseFileHeader(path)
		if err != nil {
			return nil, fmt.Errorf("parse shard %s: %w", file, err)
		}
		for _, name := range names {
			meta, ok := raw[name]
			if !ok {
				continue // index lists it, shard header doesn't — skip rather than fail the whole model
			}
			tensors = append(tensors, TensorInfo{
				Name:      name,
				Dtype:     meta.Dtype,
				Shape:     meta.Shape,
				File:      path,
				DataStart: dataBase + meta.DataOffsets[0],
				DataEnd:   dataBase + meta.DataOffsets[1],
			})
		}
	}
	sort.Slice(tensors, func(i, j int) bool { return tensors[i].Name < tensors[j].Name })
	return tensors, nil
}

// parseFileHeader reads one safetensors file's 8-byte little-endian header
// length followed by its JSON header, returning each tensor's metadata
// (keyed by name, "__metadata__" excluded) and the absolute byte offset
// where the file's data section begins.
func parseFileHeader(path string) (map[string]rawTensorMeta, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var lenBuf [8]byte
	if _, err := io.ReadFull(f, lenBuf[:]); err != nil {
		return nil, 0, fmt.Errorf("read header length: %w", err)
	}
	headerLen := binary.LittleEndian.Uint64(lenBuf[:])

	headerBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(f, headerBytes); err != nil {
		return nil, 0, fmt.Errorf("read header: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(headerBytes, &raw); err != nil {
		return nil, 0, fmt.Errorf("parse header JSON: %w", err)
	}

	tensors := make(map[string]rawTensorMeta, len(raw))
	for name, msg := range raw {
		if name == "__metadata__" {
			continue
		}
		var meta rawTensorMeta
		if err := json.Unmarshal(msg, &meta); err != nil {
			continue // skip a malformed entry rather than failing the whole file
		}
		tensors[name] = meta
	}

	return tensors, int64(8 + headerLen), nil
}
