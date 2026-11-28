#!/usr/bin/env python3
"""Validasi bahwa versi DB UAT sama dengan target migration source."""

from __future__ import annotations

import re
import subprocess
from pathlib import Path
from typing import Callable

STATUS_LINE = re.compile(r"Versi database: ([0-9]+); target: ([0-9]+)")
ROOT = Path(__file__).resolve().parent.parent


def status_is_current(return_code: int, output: str) -> bool:
    """Status hanya sah bila migrator sukses dan versinya tepat sama."""
    if return_code != 0:
        return False
    lines = output.splitlines()
    if len(lines) != 1:
        return False
    match = STATUS_LINE.fullmatch(lines[0])
    return bool(match and int(match.group(1)) == int(match.group(2)))


def check_current_status(
    runner: Callable[..., subprocess.CompletedProcess[str]] = subprocess.run,
) -> tuple[bool, str]:
    """Jalankan pemeriksaan status saja; fungsi ini tidak pernah melakukan migrasi."""
    try:
        result = runner(
            ["go", "run", "./cmd/migrate", "status"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError:
        return False, ""
    return status_is_current(result.returncode, result.stdout), result.stdout


def main() -> int:
    current, output = check_current_status()
    if not current:
        print("Status migration UAT gagal, tidak valid, atau belum mencapai target source.")
        return 1
    print(output.strip())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
