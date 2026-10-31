#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""发布新版本（整包更新）。

用法：
    python 发布新版本.py 新版本号 [更新说明] [exe路径]

例：
    python 发布新版本.py 1.1.1 "修复了xxx问题"

做的事：
    1. 把 exe（以及同目录的 使用说明.txt、规则示例.txt）打成一个 zip；
    2. 算 zip 的 sha256 和大小；
    3. 生成 latest.json：

        {
          "version":        "1.1.1",
          "notes":          "修复了xxx问题",
          "package":        "PDFFolderMover-v1.1.1.zip",
          "package_size":   2345678,
          "package_sha256": "…",
          "exe_name":       "PDF文件夹搬运助手.exe"
        }

    4. 把 zip 和 latest.json 传到同一个网址目录，
       再把 latest.json 的完整网址填进程序「⑤ 更新」即可。
"""
import hashlib
import json
import os
import sys
import zipfile

EXE_NAME = "PDF文件夹搬运助手.exe"
EXTRA_FILES = ["使用说明.txt", "规则示例.txt"]


def sha256_of(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__)
        return 2
    version = sys.argv[1].lstrip("vV")
    notes = sys.argv[2] if len(sys.argv) > 2 else ""
    here = os.path.dirname(os.path.abspath(__file__))
    exe = sys.argv[3] if len(sys.argv) > 3 else os.path.join(here, EXE_NAME)

    if not os.path.isfile(exe):
        print("找不到 exe：%s" % exe)
        return 1

    zip_name = "PDFFolderMover-v%s.zip" % version
    zip_path = os.path.join(here, zip_name)

    members = [exe]
    for extra in EXTRA_FILES:
        p = os.path.join(os.path.dirname(os.path.abspath(exe)), extra)
        if os.path.isfile(p):
            members.append(p)

    with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for p in members:
            z.write(p, os.path.basename(p))

    manifest = {
        "version": version,
        "notes": notes,
        "package": zip_name,
        "package_size": os.path.getsize(zip_path),
        "package_sha256": sha256_of(zip_path),
        "exe_name": os.path.basename(exe),
    }
    out = os.path.join(here, "latest.json")
    with open(out, "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent=2)

    print("已生成：", zip_path)
    print("已生成：", out)
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
    print("\n下一步：把 %s 和 latest.json 传到同一个网址目录，"
          "再把 latest.json 的地址填进程序「⑤ 更新」。" % zip_name)
    print("注意：新版本的版号要先在 update.go 里改 const appVersion。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
