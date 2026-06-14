from __future__ import annotations

import os
from pathlib import Path

from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parent.parent
load_dotenv(ROOT / "config.env")


def cfg(key: str, default: str = "") -> str:
    return os.getenv(key, default)


def cfg_bool(key: str, default: bool = False) -> bool:
    val = os.getenv(key, str(default)).lower()
    return val in ("1", "true", "yes", "on")


def mock_mode() -> bool:
    if cfg_bool("MOCK_MODE"):
        return True
    if cfg("TEST_PASSWORD") in ("", "CHANGE_ME") or cfg("PROD_PASSWORD") in ("", "CHANGE_ME"):
        return True
    return False


DATA_DIR = ROOT / "data"
STATE_FILE = DATA_DIR / "runs.json"
STATIC_DIR = ROOT / "static" / "dist"
