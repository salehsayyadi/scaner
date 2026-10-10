"""Retrieve the public gomobile test key, never a user's signing credential."""
import re
import subprocess
import sys
from pathlib import Path

cache = Path(subprocess.check_output(["go", "env", "GOMODCACHE"], text=True).strip())
mobile = cache / "fyne.io/fyne/v2@v2.6.3/cmd/fyne/internal/mobile"
for p in mobile.glob("*.go"):
    text = p.read_text()
    match = re.search(r"-----BEGIN RSA PRIVATE KEY-----.*?-----END RSA PRIVATE KEY-----", text, flags=re.S)
    if match:
        Path(sys.argv[1]).write_text(match.group(0) + "\n")
        print("Extracted the publicly distributed gomobile test key.")
        break
else:
    raise SystemExit("Public gomobile test key not found; refusing to invent a changing signing identity.")