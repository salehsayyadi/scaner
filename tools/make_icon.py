"""Materialize the embedded PNG as build resources; no runtime files are needed."""
import base64
from pathlib import Path
from PIL import Image

root = Path(__file__).resolve().parents[1]
out = root / "build"
out.mkdir(exist_ok=True)
png = base64.b64decode((root / "gui/assets/icon.b64").read_text().strip(), validate=True)
(out / "icon.png").write_bytes(png)
with Image.open(out / "icon.png") as image:
    image.save(out / "icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])