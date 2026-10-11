#!/usr/bin/env python3
"""Verify the explicit public source allowlist and third-party source/notices."""
import argparse
from pathlib import Path
from public_files import ROOT, verify_source


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args()
    try:
        files = verify_source(args.root)
    except ValueError as error:
        raise SystemExit(str(error))
    print(f"Public source checks passed: {len(files)} allowlisted files; notices and exact noVNC source verified")


if __name__ == "__main__":
    main()
