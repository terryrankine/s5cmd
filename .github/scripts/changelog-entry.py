#!/usr/bin/env python3
"""Insert a CHANGELOG section for an automatic patch release.

usage: changelog-entry.py vX.Y.Z "18 Sep 2026" "- bullet\n- bullet"
"""
import sys

version, date, bullets = sys.argv[1], sys.argv[2], sys.argv[3].strip()
if not bullets:
    bullets = "- dependency and toolchain updates"

path = "CHANGELOG.md"
text = open(path, encoding="utf-8").read()
entry = f"## {version} - {date}\n\n#### Dependencies and toolchain\nAutomatic patch release.\n\n{bullets}\n\n"
marker = "# Changelog\n"
if marker not in text:
    sys.exit("CHANGELOG.md does not start with '# Changelog'")
open(path, "w", encoding="utf-8", newline="\n").write(text.replace(marker, marker + entry, 1))
print(f"added {version}")
