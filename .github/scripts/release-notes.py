#!/usr/bin/env python3
"""Print the CHANGELOG section for a tag (e.g. v2.4.2), or nothing."""
import re
import sys

tag = sys.argv[1]
text = open("CHANGELOG.md", encoding="utf-8").read()
match = re.search(r"^## " + re.escape(tag) + r" [^\n]*\n(.*?)(?=^## |\Z)", text, flags=re.S | re.M)
print(match.group(1).strip() if match else "")
