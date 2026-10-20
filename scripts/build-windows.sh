#!/bin/bash
# Build the x64 portable package from macOS/Linux with an installed Go toolchain.
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$TASK_ROOT"
./scripts/build.sh -platform windows/amd64
python3 - <<'PY'
# -*- coding: utf-8 -*-
from pathlib import Path
import json
import zipfile

root = Path.cwd()
version = json.loads((root / 'mygo.json').read_text())['version']
name = json.loads((root / 'mygo.json').read_text())['name']
stage = root / 'build' / 'windows-amd64'
archive = stage.parent / f'{name} {version} Windows x64.zip'
readme = (root / 'docs' / 'windows-package.txt').read_text(encoding='utf-8')
(stage / '使用说明.txt').write_text(readme, encoding='utf-8-sig')
with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as package:
    for filename in [f'{name}.exe', 'icon.png', 'tray.png', 'world-land.json', '使用说明.txt']:
        path = stage / filename
        package.write(path, f'{name}/{filename}')
with zipfile.ZipFile(archive) as package:
    bad = package.testzip()
    if bad:
        raise RuntimeError(f'ZIP integrity check failed: {bad}')
print(f'Portable package: {archive} ({archive.stat().st_size / 1024 / 1024:.1f} MiB)')
PY
