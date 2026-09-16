#!/usr/bin/env python3
"""Build the Jerbert app icon for the Mac sync app from web/public/logo.svg.

    make icon

Lays Jerbert out in the macOS app icon shape (a rounded square with a
transparent margin, as every app has used since Big Sur) in the PWA icon's
colours, and writes internal/server/jerbert.icns for the installer to fetch.
Re-run it whenever Jerbert changes. Needs rsvg-convert and Pillow.
"""
import io
import subprocess
import sys
from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[2]
SVG = ROOT / "web/public/logo.svg"
OUT = ROOT / "internal/server/jerbert.icns"

BACKGROUND = (0x13, 0x13, 0x13, 255)  # sampled from web/public/icon-512.png
TEAL = (0x2E, 0xC4, 0xB6, 255)

CANVAS = 1024
TILE = 824        # Apple's grid: 824px tile centred on a 1024px canvas
RADIUS = 185      # close match to the Big Sur corner
GLYPH_WIDTH = 0.64  # Jerbert's width as a fraction of the tile


def render_glyph(width: int) -> Image.Image:
    png = subprocess.run(["rsvg-convert", "-w", str(width), str(SVG)],
                         check=True, capture_output=True).stdout
    shape = Image.open(io.BytesIO(png)).convert("RGBA")
    alpha = shape.getchannel("A")
    shape = shape.crop(alpha.getbbox())  # centre what's drawn, not the SVG's own padding
    teal = Image.new("RGBA", shape.size, TEAL)
    teal.putalpha(shape.getchannel("A"))
    return teal


def main() -> None:
    scale = 4  # draw big and shrink, for smooth corners
    tile = Image.new("RGBA", (CANVAS * scale, CANVAS * scale), (0, 0, 0, 0))
    offset = (CANVAS - TILE) // 2 * scale
    ImageDraw.Draw(tile).rounded_rectangle(
        (offset, offset, offset + TILE * scale - 1, offset + TILE * scale - 1),
        radius=RADIUS * scale, fill=BACKGROUND)
    tile = tile.resize((CANVAS, CANVAS), Image.LANCZOS)

    target = round(TILE * GLYPH_WIDTH)
    glyph = render_glyph(4096)
    glyph = glyph.resize((target, round(glyph.height * target / glyph.width)), Image.LANCZOS)
    tile.alpha_composite(glyph, ((CANVAS - glyph.width) // 2, (CANVAS - glyph.height) // 2))

    tile.save(OUT, format="ICNS")
    print(f"wrote {OUT.relative_to(ROOT)}")


if __name__ == "__main__":
    sys.exit(main())
