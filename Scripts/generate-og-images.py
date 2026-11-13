#!/usr/bin/env python3
"""Render website/static/img/og-<code>.png, one 1200x630 share image per website locale.

Local tools are read from the repository's .env (git-ignored), or the environment:

    OG_CHROME=   Chrome or Chromium executable
    OG_MAGICK=   ImageMagick 7 `magick` executable
    OG_FONTS=    folder with Source Han Serif OTC SourceHanSerif-{Heavy,SemiBold,Medium}.ttc

Every other input comes from the repository: the hero copy in website/locales, the main panel
screenshots in docs/images, the app icon and provider icons in internal/app/assets and the
platform icons in website/icons.
"""

import json
import os
import re
import struct
import subprocess
import sys
import tempfile
from pathlib import Path


REPO = Path(__file__).resolve().parents[1]
OUT = REPO / "website" / "static" / "img"
# Faces of the Source Han Serif collection, in its order: Japanese, Korean, Simplified and Traditional Chinese.
FACES = {"J": 0, "K": 1, "SC": 2, "TC": 3}
WEIGHTS = ("Heavy", "SemiBold", "Medium")
FACE = {"en": "J", "fr": "J", "ru": "J", "ja": "J", "ko": "K", "zh-hans": "SC", "zh-hant": "TC"}
LANG = {"en": "en", "fr": "fr", "ru": "ru", "ja": "ja", "ko": "ko", "zh-hans": "zh-Hans", "zh-hant": "zh-Hant"}

PAGE = """<!doctype html><html lang="{lang}"><head><meta charset="utf-8"><style>
@font-face{{font-family:H;src:url(SHS-{face}-Heavy.otf)}}
@font-face{{font-family:S;src:url(SHS-{face}-SemiBold.otf)}}
@font-face{{font-family:M;src:url(SHS-{face}-Medium.otf)}}
*{{box-sizing:border-box}}
html,body{{margin:0;width:1200px;height:630px;overflow:hidden}}
.og{{position:relative;width:1200px;height:630px;overflow:hidden;color:#2a211c;
  background:
    radial-gradient(760px 560px at 100% 0%,rgb(224 138 85/.42),transparent 62%),
    radial-gradient(520px 420px at 0% 100%,rgb(181 88 46/.10),transparent 65%),
    #f7f3ee}}
.og::before{{content:"";position:absolute;inset:0;
  background-image:radial-gradient(rgb(108 95 86/.20) 1.3px,transparent 1.5px);background-size:24px 24px;
  -webkit-mask-image:linear-gradient(100deg,#000 0%,transparent 55%)}}
.left{{position:absolute;left:72px;top:60px;bottom:56px;width:600px;display:flex;flex-direction:column}}
.brand{{display:flex;align-items:center;gap:18px}}
.brand img{{width:96px;height:96px;margin-left:-6px;filter:drop-shadow(0 6px 10px rgb(120 60 25/.22))}}
.name{{font-family:H;font-size:94px;line-height:1;letter-spacing:-1.5px;padding-bottom:6px}}
.ety{{margin-top:16px;font-family:M;font-size:25px;color:#6c5f56;white-space:nowrap}}
.ety b{{font-family:H;font-weight:normal;color:#b5582e}}
.rule{{width:64px;height:5px;border-radius:3px;background:#b5582e;margin:28px 0 24px}}
.tag{{font-family:S;font-size:var(--t,38px);line-height:1.45;text-wrap:balance;word-break:auto-phrase}}
:lang(zh) .tag,:lang(ko) .tag{{word-break:keep-all}}
.foot{{margin-top:auto;padding-top:20px}}
.pills{{display:flex;gap:12px}}
.pill{{display:flex;flex-shrink:0;white-space:nowrap;align-items:center;gap:9px;height:46px;padding:0 18px 0 13px;border-radius:999px;
  background:#fffdfa;border:1.5px solid #e2d7cc;font-family:S;font-size:var(--p,21px);box-shadow:0 2px 6px rgb(60 40 25/.06)}}
.pill img{{width:24px;height:24px}}
.meta{{display:flex;align-items:center;gap:10px;margin-top:18px;font-family:M;font-size:21px;color:#6c5f56}}
.meta svg{{width:23px;height:23px}}
.meta .dot{{margin:0 8px;color:#c9b8a8}}
.meta .url{{font-family:S;color:#a14d27}}
.back,.front{{position:absolute;border-radius:30px;overflow:hidden;background:#1b1c1f;
  box-shadow:0 0 0 9px #2c2d33,0 34px 70px rgb(60 35 20/.32)}}
.back{{left:790px;top:120px;width:350px;transform:rotate(9deg);filter:brightness(.72) saturate(.8)}}
.front{{left:736px;top:66px;width:384px;transform:rotate(3deg)}}
.back img,.front img{{display:block;width:100%}}
</style></head><body><div class="og">
<div class="back"><img src="{back}"></div>
<div class="front"><img src="{front}"></div>
<div class="left">
  <div class="brand"><img src="{app_icon}"><div class="name">Quosparr</div></div>
  <div class="ety">{ety}</div>
  <div class="rule"></div>
  <div class="tag">{tag}</div>
  <div class="foot">
    <div class="pills">
      <div class="pill"><img src="{codex}">Codex</div>
      <div class="pill"><img src="{claude}">Claude</div>
      <div class="pill"><img src="{gemini}">Gemini</div>
      <div class="pill"><img src="{grok}">Grok</div>
    </div>
    <div class="meta">{apple}{windows}{linux}<span>macOS · Windows · Linux</span><span class="dot">/</span><span class="url">quosparr.com</span></div>
  </div>
</div></div>
<script>
document.fonts.ready.then(()=>{{
  const left=document.querySelector('.left'),tag=document.querySelector('.tag'),ety=document.querySelector('.ety');
  for(let s=25;s>17&&ety.scrollWidth>left.clientWidth;s--)ety.style.fontSize=s+'px';
  for(let s=38;s>26&&left.scrollHeight>left.clientHeight;s--)tag.style.setProperty('--t',s+'px');
  const pills=document.querySelector('.pills');
  for(let s=21;s>15&&pills.scrollWidth>left.clientWidth;s--)pills.style.setProperty('--p',s+'px');
}});
</script></body></html>"""


def config() -> dict[str, str]:
    env = {}
    path = REPO / ".env"
    if path.exists():
        for line in path.read_text().splitlines():
            key, sep, value = line.partition("=")
            if sep and not key.strip().startswith("#"):
                env[key.strip()] = value.strip().strip("\"'")
    env.update({key: value for key, value in os.environ.items() if key.startswith("OG_")})
    missing = [key for key in ("OG_CHROME", "OG_MAGICK", "OG_FONTS") if not env.get(key)]
    if missing:
        sys.exit(f"Set {', '.join(missing)} in {path.name} at the repository root")
    return env


def collection_face(data: bytes, index: int) -> bytes:
    """One face of an OpenType collection as a standalone font; shared tables are copied into it."""
    if data[:4] != b"ttcf":
        raise ValueError("not an OpenType collection")
    start = struct.unpack_from(">I", data, 12 + 4 * index)[0]
    count = struct.unpack_from(">H", data, start + 4)[0]
    header, body = bytearray(data[start:start + 12]), bytearray()
    for i in range(count):
        tag, checksum, offset, length = struct.unpack_from(">4sIII", data, start + 12 + 16 * i)
        header += struct.pack(">4sIII", tag, checksum, 12 + 16 * count + len(body), length)
        body += data[offset:offset + length] + b"\0" * (-length % 4)
    return bytes(header + body)


def split_fonts(folder: Path, work: Path) -> None:
    for weight in WEIGHTS:
        data = (folder / f"SourceHanSerif-{weight}.ttc").read_bytes()
        for face, index in FACES.items():
            (work / f"SHS-{face}-{weight}.otf").write_bytes(collection_face(data, index))


def icon(name: str) -> str:
    svg = (REPO / "website" / "icons" / f"{name}.svg").read_text()
    svg = re.sub(r"<title>.*?</title>", "", svg)
    return svg.replace("<svg ", '<svg fill="currentColor" ', 1)


def main() -> None:
    env = config()
    providers = REPO / "internal" / "app" / "assets" / "providers"
    images = REPO / "docs" / "images"
    assets = {
        "app_icon": (REPO / "internal" / "app" / "assets" / "app-icon.png").as_uri(),
        **{name: (providers / file).as_uri() for name, file in
           {"codex": "codex-color.svg", "claude": "claude-color.svg", "gemini": "gemini-color.svg", "grok": "grok.svg"}.items()},
        "apple": icon("apple-fill"), "windows": icon("windows-fill"), "linux": icon("terminal-box-fill"),
    }
    with tempfile.TemporaryDirectory() as tmp:
        work = Path(tmp)
        split_fonts(Path(env["OG_FONTS"]), work)
        for code, face in FACE.items():
            hero = json.loads((REPO / "website" / "locales" / f"{code}.json").read_text())["hero"]
            # The locale's own main panel in front; the other language's panel tilts behind it.
            front, back = ("panel.webp", "panel-en.webp") if code.startswith("zh-") else ("panel-en.webp", "panel.webp")
            page = work / f"og-{code}.html"
            page.write_text(PAGE.format(lang=LANG[code], face=face, ety=hero["etymology"], tag=hero["tagline"],
                                        front=(images / front).as_uri(), back=(images / back).as_uri(), **assets))
            raw = work / f"og-{code}@2x.png"
            subprocess.run([env["OG_CHROME"], "--headless=new", "--disable-gpu", "--hide-scrollbars",
                            "--force-device-scale-factor=2", "--window-size=1200,630", "--virtual-time-budget=8000",
                            "--allow-file-access-from-files", f"--screenshot={raw}", page.as_uri()],
                           check=True, capture_output=True)
            png = OUT / f"og-{code}.png"
            subprocess.run([env["OG_MAGICK"], str(raw), "-resize", "1200x630", "-strip",
                            "-define", "png:compression-level=9", str(png)], check=True, capture_output=True)
            print(png.relative_to(REPO))


if __name__ == "__main__":
    main()
