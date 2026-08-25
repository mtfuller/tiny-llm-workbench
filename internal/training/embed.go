package training

import _ "embed"

// trainScript is the bundled Python script that drives mlx-lm's LoRA
// trainer and reports progress as JSON lines on stdout. See scripts/train.py.
//
//go:embed scripts/train.py
var trainScript string
