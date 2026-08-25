#!/usr/bin/env python3
"""LoRA fine-tuning driver for Tiny LLM Workbench.

This wraps mlx-lm's `lora` training CLI (`python -m mlx_lm.lora`) as a
subprocess, re-emitting its progress as JSON lines on stdout so the Go
webserver (internal/training) can parse it and stream it to the browser UI.

Each output line is one JSON object, one of:
  {"type": "progress", "iteration": N, "trainLoss": F, "valLoss": F,
   "peakMemGB": F, "tokensPerSec": F}   (fields other than "iteration" are
                                          omitted when mlx-lm didn't report them
                                          on that line)
  {"type": "done"}
  {"type": "error", "message": "..."}

NOTE: this script was written against mlx-lm's documented CLI and typical
log output format, but has not been run against a real mlx-lm install while
developing it (no working MLX environment was available) — the exact log
line format may drift across mlx-lm versions. If progress stops appearing
while training clearly is running, check the regexes below against your
installed mlx-lm's actual output.
"""

import argparse
import json
import re
import subprocess
import sys

# Examples of the mlx-lm log lines this expects to parse:
#   "Iter 10: Train loss 1.234, Learning Rate 1.000e-05, It/sec 0.523, " \
#   "Tokens/sec 123.456, Trained Tokens 1000, Peak mem 4.567 GB"
#   "Iter 10: Val loss 1.456, Val took 12.345s"
TRAIN_LINE = re.compile(
    r"Iter (?P<iter>\d+): Train loss (?P<loss>[\d.]+).*?"
    r"(?:Tokens/sec (?P<tps>[\d.]+))?.*?"
    r"(?:Peak mem (?P<mem>[\d.]+) GB)?"
)
VAL_LINE = re.compile(r"Iter (?P<iter>\d+): Val loss (?P<loss>[\d.]+)")


def emit(**fields):
    print(json.dumps(fields), flush=True)


def parse_args():
    p = argparse.ArgumentParser()
    p.add_argument("--model", required=True, help="HF repo id or local path to an MLX-format model")
    p.add_argument("--data-dir", required=True, help="directory with train.jsonl and valid.jsonl")
    p.add_argument("--output-dir", required=True, help="where to write the trained LoRA adapter")
    p.add_argument("--iters", type=int, required=True)
    p.add_argument("--learning-rate", type=float, default=None)
    return p.parse_args()


def main():
    args = parse_args()

    cmd = [
        sys.executable, "-m", "mlx_lm.lora",
        "--model", args.model,
        "--train",
        "--data", args.data_dir,
        "--iters", str(args.iters),
        "--adapter-path", args.output_dir,
    ]
    if args.learning_rate:
        cmd += ["--learning-rate", str(args.learning_rate)]

    try:
        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1)
    except FileNotFoundError as e:
        emit(type="error", message=f"failed to launch mlx_lm.lora: {e}")
        sys.exit(1)

    last_iter = 0
    for line in proc.stdout:
        line = line.strip()

        m = TRAIN_LINE.search(line)
        if m:
            last_iter = int(m.group("iter"))
            emit(
                type="progress",
                iteration=last_iter,
                trainLoss=float(m.group("loss")),
                tokensPerSec=float(m.group("tps")) if m.group("tps") else None,
                peakMemGB=float(m.group("mem")) if m.group("mem") else None,
            )
            continue

        m = VAL_LINE.search(line)
        if m:
            last_iter = int(m.group("iter"))
            emit(type="progress", iteration=last_iter, valLoss=float(m.group("loss")))
            continue

    returncode = proc.wait()
    if returncode != 0:
        emit(type="error", message=f"mlx_lm.lora exited with status {returncode}")
        sys.exit(returncode)

    emit(type="done")


if __name__ == "__main__":
    main()
