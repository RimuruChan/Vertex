#!/usr/bin/env python3
"""Vertex testlib/Kattis adapter; original binary and its finalizers run unchanged."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

root = Path(__file__).resolve().parent
config = json.loads((root / "vertex-adapter.json").read_text(encoding="utf-8"))
command = [str(root / "vertex_original")]
if config["role"] == "input-validator":
    result = subprocess.run(command + config["arguments"] + sys.argv[1:])
    sys.exit(42 if result.returncode == 0 else 43)
if len(sys.argv) < 4:
    sys.exit(44)
input_file, answer_file, feedback = sys.argv[1:4]
output_name = None
try:
    with tempfile.NamedTemporaryFile(prefix="vertex-output-", dir=feedback, delete=False) as output:
        output_name = output.name
        total = 0
        while True:
            block = sys.stdin.buffer.read(8192)
            if not block:
                break
            total += len(block)
            if total > 32 * 1024 * 1024:
                sys.exit(44)
            output.write(block)
    # testlib diagnostics may contain expected answers: keep them jury-only.
    fd = os.open(str(Path(feedback) / "judgemessage.txt"), os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as jury:
        result = subprocess.run(command + [input_file, output_name, answer_file] + config["arguments"] + sys.argv[4:], stderr=jury)
    code = result.returncode
    sys.exit(42 if code == 0 else 43 if code in (1, 2, 4, 8) else 44)
except (OSError, ValueError):
    sys.exit(44)
finally:
    if output_name is not None:
        os.unlink(output_name)
