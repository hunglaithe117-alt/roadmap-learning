#!/usr/bin/env python3
"""T8.1 step 1 — FetchHSK: tai wordlist HSK 2.0 (drkameleon chinh, clem109 backup),
kiem tra count + diff, luu raw JSON de seed_hsk.py dung.
Nguon: MIT (giữ copyright/attribution trong memory + THIRD-PARTY-LICENSES).
"""
import json
import subprocess
import sys
import urllib.request
from pathlib import Path

RAW = Path(__file__).resolve().parent / "hsk_raw"
DRK = "https://raw.githubusercontent.com/drkameleon/complete-hsk-vocabulary/main/wordlists/exclusive/old/{n}.json"
CLEM_REPO = "https://github.com/clem109/hsk-vocabulary.git"

# Count chuan HSK 2.0 (so dong ke ca bien the da am / dong trung trong nguon).
EXPECT_UNIQUE = {1: 150, 2: 147, 3: 298, 4: 598}
EXPECT_ROWS = {1: 150, 2: 150, 3: 299, 4: 601}  # clem109 backup (ke ca dong da am)


def fetch(url, dest):
    req = urllib.request.Request(url, headers={"User-Agent": "lang-learn-app/T8.1"})
    with urllib.request.urlopen(req, timeout=60) as r, open(dest, "wb") as f:
        f.write(r.read())


def main():
    RAW.mkdir(parents=True, exist_ok=True)
    for n in (1, 2, 3, 4):
        dest = RAW / f"drk-old-{n}.json"
        if not dest.exists():
            fetch(DRK.format(n=n), dest)
        data = json.loads(dest.read_text(encoding="utf-8"))
        uniq = len({e["simplified"] for e in data})
        assert uniq == EXPECT_UNIQUE[n], f"drkameleon L{n}: {uniq} != {EXPECT_UNIQUE[n]}"
        print(f"drkameleon old-{n}: {len(data)} entries, {uniq} unique hanzi OK")
    clem = RAW / "clem109"
    if not (clem / "hsk-vocab-json").exists():
        subprocess.run(["git", "clone", "--depth", "1", CLEM_REPO, str(clem)], check=True)
    for n in (1, 2, 3, 4):
        data = json.loads((clem / f"hsk-vocab-json/hsk-level-{n}.json").read_text(encoding="utf-8"))
        uniq = {e["hanzi"] for e in data}
        assert len(data) == EXPECT_ROWS[n], f"clem109 L{n}: {len(data)} != {EXPECT_ROWS[n]}"
        drk = json.loads((RAW / f"drk-old-{n}.json").read_text(encoding="utf-8"))
        drkset = {e["simplified"] for e in drk}
        assert uniq == drkset, f"diff hanzi set L{n}: {uniq ^ drkset}"
        print(f"clem109 L{n}: {len(data)} rows, {len(uniq)} unique hanzi OK (set == drkameleon)")
    print("FetchHSK OK: 1193 unique hanzi L1-L4 (1200 rows ke ca bien the da am).")


if __name__ == "__main__":
    sys.exit(main())
