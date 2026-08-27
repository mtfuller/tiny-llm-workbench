package safetensors

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
)

// defaultGridSize matches the design doc's suggested fixed subsample size.
const defaultGridSize = 200

// HeatmapData is a tensor subsampled down to a fixed grid, plus statistics
// computed over every element (not just the sampled ones), for the Models
// page's layer heatmap visualizer.
type HeatmapData struct {
	Rows int       `json:"rows"`
	Cols int       `json:"cols"`
	Grid []float32 `json:"grid"` // row-major, length Rows*Cols
	Min  float32   `json:"min"`
	Max  float32   `json:"max"`
	Mean float32   `json:"mean"`
	Std  float32   `json:"std"`
}

// ExtractHeatmap reads t's raw bytes (and only t's bytes — via a single
// os.File.ReadAt at its exact offsets, never the whole model file),
// decodes them per its dtype, and returns a gridSize x gridSize (or
// smaller, for a tensor with fewer rows/cols than that) subsample plus
// summary statistics over the full tensor. gridSize <= 0 uses
// defaultGridSize.
func ExtractHeatmap(t TensorInfo, gridSize int) (HeatmapData, error) {
	if gridSize <= 0 {
		gridSize = defaultGridSize
	}

	elemSize, decode, err := decoderFor(t.Dtype)
	if err != nil {
		return HeatmapData{}, err
	}

	length := t.DataEnd - t.DataStart
	if length <= 0 || length%int64(elemSize) != 0 {
		return HeatmapData{}, fmt.Errorf("tensor %q has an invalid byte range for dtype %s", t.Name, t.Dtype)
	}

	f, err := os.Open(t.File)
	if err != nil {
		return HeatmapData{}, err
	}
	defer f.Close()

	buf := make([]byte, length)
	if _, err := f.ReadAt(buf, t.DataStart); err != nil {
		return HeatmapData{}, fmt.Errorf("read tensor data: %w", err)
	}

	n := int64(len(buf)) / int64(elemSize)
	rows, cols := matrixShape(t.Shape)
	if rows*cols != n {
		// The declared shape doesn't match the actual byte count (shouldn't
		// happen for a well-formed file) — fall back to a flat view rather
		// than failing the request.
		rows, cols = 1, n
	}

	outRows := int64(gridSize)
	if outRows > rows {
		outRows = rows
	}
	outCols := int64(gridSize)
	if outCols > cols {
		outCols = cols
	}

	var sum, sumSq float64
	minV := float32(math.Inf(1))
	maxV := float32(math.Inf(-1))
	for i := int64(0); i < n; i++ {
		v := decode(buf[i*int64(elemSize) : i*int64(elemSize)+int64(elemSize)])
		fv := float64(v)
		sum += fv
		sumSq += fv * fv
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	mean := float32(sum / float64(n))
	variance := sumSq/float64(n) - float64(mean)*float64(mean)
	if variance < 0 {
		variance = 0
	}
	std := float32(math.Sqrt(variance))

	grid := make([]float32, outRows*outCols)
	rowStride := float64(rows) / float64(outRows)
	colStride := float64(cols) / float64(outCols)
	for r := int64(0); r < outRows; r++ {
		srcRow := int64(float64(r) * rowStride)
		for c := int64(0); c < outCols; c++ {
			srcCol := int64(float64(c) * colStride)
			idx := srcRow*cols + srcCol
			if idx >= n {
				idx = n - 1
			}
			grid[r*outCols+c] = decode(buf[idx*int64(elemSize) : idx*int64(elemSize)+int64(elemSize)])
		}
	}

	return HeatmapData{Rows: int(outRows), Cols: int(outCols), Grid: grid, Min: minV, Max: maxV, Mean: mean, Std: std}, nil
}

// matrixShape collapses an N-dimensional shape into a 2D (rows, cols) view
// for heatmap purposes: the first dimension stays rows, every remaining
// dimension is flattened into cols.
func matrixShape(shape []int64) (rows, cols int64) {
	if len(shape) == 0 {
		return 1, 1
	}
	if len(shape) == 1 {
		return 1, shape[0]
	}
	rows = shape[0]
	cols = 1
	for _, d := range shape[1:] {
		cols *= d
	}
	return rows, cols
}

// decoderFor returns the byte width and a decode function for dtype.
// Quantized/packed and integer dtypes aren't supported (a heatmap of raw
// packed quantization bytes wouldn't mean anything) — callers should show
// the caller a clear "can't visualize this dtype" message rather than
// garbage output.
func decoderFor(dtype string) (elemSize int, decode func([]byte) float32, err error) {
	switch strings.ToUpper(dtype) {
	case "F32":
		return 4, func(b []byte) float32 {
			return math.Float32frombits(binary.LittleEndian.Uint32(b))
		}, nil
	case "F16":
		return 2, func(b []byte) float32 {
			return float16ToFloat32(binary.LittleEndian.Uint16(b))
		}, nil
	case "BF16":
		return 2, func(b []byte) float32 {
			// bfloat16 is simply a truncated float32: its 16 bits are the
			// top 16 bits (sign + exponent + 7 mantissa bits) of a float32.
			return math.Float32frombits(uint32(binary.LittleEndian.Uint16(b)) << 16)
		}, nil
	default:
		return 0, nil, fmt.Errorf("unsupported dtype for heatmap: %s", dtype)
	}
}

// float16ToFloat32 converts an IEEE 754 binary16 bit pattern to float32,
// handling zero, subnormals, and inf/NaN. exp is signed (int32, not
// uint32): normalizing a subnormal decrements it below zero before it's
// rebiased for float32, and a uint32 would silently wrap around instead.
func float16ToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := int32(h&0x7C00) >> 10
	frac := uint32(h & 0x03FF)

	switch exp {
	case 0:
		if frac == 0 {
			return math.Float32frombits(sign)
		}
		// Subnormal: normalize by shifting left until the implicit leading
		// bit appears, adjusting the exponent to match.
		exp = 1
		for frac&0x0400 == 0 {
			frac <<= 1
			exp--
		}
		frac &= 0x03FF
	case 0x1F:
		return math.Float32frombits(sign | 0x7F800000 | frac<<13)
	}

	exp32 := uint32(exp + (127 - 15))
	return math.Float32frombits(sign | exp32<<23 | frac<<13)
}
