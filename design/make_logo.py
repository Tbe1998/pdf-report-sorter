#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""PDF 日报分拣助手 —— 原创图标（手写 SVG + Chromium 栅格化）

不用 AI 生图、不用素材：路径全部手写，1024 渲染后按 LANCZOS 缩到各尺寸。
"""
import os
import sys

from PIL import Image, ImageDraw
from playwright.sync_api import sync_playwright

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "icons")
SIZES = [512, 256, 128, 64, 48, 32, 16]

# ---------------------------------------------------------------- 方案 A
# 深青底 + 白纸折角 + 三条分流轨道 → "一份日报，分到各项目"
A = """
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0.6" y2="1">
      <stop offset="0" stop-color="#0F6675"/>
      <stop offset="1" stop-color="#08313A"/>
    </linearGradient>
    <linearGradient id="paper" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#FFFFFF"/>
      <stop offset="1" stop-color="#E9F2F3"/>
    </linearGradient>
  </defs>
  <rect width="1024" height="1024" rx="228" fill="url(#bg)"/>
  <path d="M292 158 H596 L716 278 V584 H292 Z" fill="url(#paper)"/>
  <path d="M596 158 L716 278 H596 Z" fill="#B7D5DA"/>
  <rect x="352" y="352" width="272" height="28" rx="14" fill="#9BBCC3"/>
  <rect x="352" y="424" width="204" height="28" rx="14" fill="#9BBCC3"/>
  <g fill="none" stroke-width="60" stroke-linecap="round">
    <path d="M504 584 C504 690 288 676 288 762" stroke="#FFB020"/>
    <path d="M504 584 L504 762" stroke="#3BD6C2"/>
    <path d="M504 584 C504 690 720 676 720 762" stroke="#FF7F70"/>
  </g>
  <rect x="238" y="748" width="100" height="92" rx="30" fill="#FFB020"/>
  <rect x="454" y="748" width="100" height="92" rx="30" fill="#3BD6C2"/>
  <rect x="670" y="748" width="100" height="92" rx="30" fill="#FF7F70"/>
</svg>
"""

# ---------------------------------------------------------------- 方案 B
# 浅底 + 强描边纸 + 分叉（最清晰，16px 也不糊）
B = """
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
  <defs>
    <linearGradient id="bg2" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#F7FAFB"/>
      <stop offset="1" stop-color="#DCE9EC"/>
    </linearGradient>
  </defs>
  <rect width="1024" height="1024" rx="228" fill="url(#bg2)"/>
  <path d="M310 170 H580 L700 290 V600 H310 Z" fill="#FFFFFF" stroke="#103B45"
        stroke-width="46" stroke-linejoin="round"/>
  <path d="M580 170 L700 290 H580 Z" fill="#BBD8DD" stroke="#103B45"
        stroke-width="46" stroke-linejoin="round"/>
  <rect x="372" y="350" width="230" height="26" rx="13" fill="#103B45"/>
  <rect x="372" y="424" width="180" height="26" rx="13" fill="#103B45" opacity="0.55"/>
  <g fill="none" stroke="#0E8F9B" stroke-width="58" stroke-linecap="round">
    <path d="M505 600 L505 690"/>
    <path d="M505 690 C505 760 350 750 350 826"/>
    <path d="M505 690 C505 760 660 750 660 826"/>
  </g>
  <rect x="302" y="806" width="96" height="86" rx="26" fill="#0E8F9B"/>
  <rect x="612" y="806" width="96" height="86" rx="26" fill="#0E8F9B"/>
</svg>
"""

# ---------------------------------------------------------------- 方案 C
# 三页纸错位叠放 + 一条强调色条 → 多项目
C = """
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
  <defs>
    <linearGradient id="bg3" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#2A3573"/>
      <stop offset="1" stop-color="#161C3E"/>
    </linearGradient>
  </defs>
  <rect width="1024" height="1024" rx="228" fill="url(#bg3)"/>
  <g opacity="0.55">
    <rect x="152" y="280" width="330" height="450" rx="44" fill="#5B6BFF"/>
    <rect x="542" y="280" width="330" height="450" rx="44" fill="#33D6C0"/>
  </g>
  <rect x="342" y="210" width="340" height="470" rx="44" fill="#FFFFFF"/>
  <rect x="402" y="300" width="220" height="26" rx="13" fill="#C3CCE6"/>
  <rect x="402" y="368" width="170" height="26" rx="13" fill="#C3CCE6"/>
  <rect x="402" y="436" width="200" height="26" rx="13" fill="#C3CCE6"/>
  <rect x="402" y="552" width="180" height="56" rx="28" fill="#FFB020"/>
</svg>
"""

# ---------------------------------------------------------------- 方案 D
# 圆形徽章 + 纸 + 向右箭头（"归档/送出"）
D = """
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
  <defs>
    <linearGradient id="bg4" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#0E9AA7"/>
      <stop offset="1" stop-color="#07606B"/>
    </linearGradient>
  </defs>
  <circle cx="512" cy="512" r="486" fill="url(#bg4)"/>
  <path d="M268 250 H508 L614 356 V636 H268 Z" fill="#FFFFFF"/>
  <path d="M508 250 L614 356 H508 Z" fill="#BFE0E4"/>
  <rect x="322" y="360" width="200" height="24" rx="12" fill="#A9C9CE"/>
  <rect x="322" y="418" width="160" height="24" rx="12" fill="#A9C9CE"/>
  <rect x="322" y="476" width="220" height="24" rx="12" fill="#A9C9CE"/>
  <path d="M560 690 H760 V636 L900 736 L760 836 V782 H560 Z" fill="#FFB020"/>
</svg>
"""

VARIANTS = {"A": A, "B": B, "C": C, "D": D}


def render_all():
    os.makedirs(OUT, exist_ok=True)
    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        page = browser.new_page(viewport={"width": 1024, "height": 1024}, device_scale_factor=1)
        for name, svg in VARIANTS.items():
            page.set_content(
                f"<html><body style='margin:0;background:transparent'>{svg}</body></html>"
            )
            page.wait_for_timeout(120)
            png = os.path.join(OUT, f"{name}-1024.png")
            page.screenshot(path=png, omit_background=True,
                            clip={"x": 0, "y": 0, "width": 1024, "height": 1024})
            src = Image.open(png).convert("RGBA")
            for s in SIZES:
                src.resize((s, s), Image.LANCZOS).save(os.path.join(OUT, f"{name}-{s}.png"))
            src.resize((256, 256), Image.LANCZOS).save(
                os.path.join(OUT, f"{name}.ico"),
                sizes=[(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (16, 16)],
            )
            print("ok", name)
        browser.close()


def preview():
    names = list(VARIANTS)
    cell = 230
    W, H = cell * len(names) + 40, 330
    for bgc, tag in (("#F1F4F6", "浅底"), ("#1B2430", "深底")):
        sheet = Image.new("RGB", (W, H), bgc)
        d = ImageDraw.Draw(sheet)
        for i, n in enumerate(names):
            x = 20 + i * cell
            im = Image.open(os.path.join(OUT, f"{n}-256.png")).convert("RGBA").resize((170, 170))
            sheet.paste(im, (x + 25, 60), im)
            d.text((x + 25, 26), f"方案 {n}", fill="#12333C" if tag == "浅底" else "#DDE6EE")
            xx = x + 12
            for s in (64, 48, 32, 16):
                im2 = Image.open(os.path.join(OUT, f"{n}-{s}.png")).convert("RGBA")
                sheet.paste(im2, (xx, 248), im2)
                xx += s + 12
        sheet.save(os.path.join(OUT, f"预览-{tag}.png"))
    print("预览完成")


if __name__ == "__main__":
    render_all()
    preview()
