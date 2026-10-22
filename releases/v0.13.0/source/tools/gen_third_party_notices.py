"""生成 THIRD-PARTY-NOTICES.txt：收集实际链接的 Go 模块与前端生产依赖的许可文本。

只读取本地模块缓存与 node_modules，不联网。发布前重新生成一次即可。

用法：
    python -I tools/gen_third_party_notices.py
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "THIRD-PARTY-NOTICES.txt"
LICENSE_NAMES = ("LICENSE", "LICENSE.txt", "LICENSE.md", "LICENCE", "COPYING",
                 "COPYING.txt", "NOTICE", "NOTICE.txt", "PATENTS")


def run(cmd: list[str], cwd: Path) -> str:
    # Windows 上 npm 是 .cmd 包装脚本，CreateProcess 不能直接执行。
    if os.name == "nt" and cmd and cmd[0] == "npm":
        cmd = ["cmd", "/c", *cmd]
    proc = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True,
                          encoding="utf-8", errors="replace")
    if proc.returncode != 0:
        raise SystemExit(f"命令失败: {' '.join(cmd)}\n{proc.stderr.strip()}")
    return proc.stdout


def read_license_texts(directory: Path) -> str:
    parts: list[str] = []
    seen: set[str] = set()
    for name in LICENSE_NAMES:
        for candidate in sorted(directory.glob(name + "*")):
            if not candidate.is_file() or candidate.name in seen:
                continue
            seen.add(candidate.name)
            try:
                text = candidate.read_text(encoding="utf-8", errors="replace").strip()
            except OSError:
                continue
            if text:
                parts.append(f"--- {candidate.name} ---\n{text}")
    return "\n\n".join(parts)


def go_modules() -> list[tuple[str, str, str]]:
    go = "go"
    listing = run([go, "list", "-deps", "-f",
                   "{{if .Module}}{{.Module.Path}}@{{.Module.Version}}{{end}}",
                   "./cmd/agentdeck-desktop"], ROOT)
    modules: list[tuple[str, str, str]] = []
    for line in sorted(set(listing.split())):
        if not line or "@" not in line:
            continue
        path, version = line.split("@", 1)
        if path == "github.com/enpiaohj/agentdeck" or not version:
            continue
        directory = run([go, "list", "-m", "-f", "{{.Dir}}", path], ROOT).strip()
        text = read_license_texts(Path(directory)) if directory else ""
        modules.append((path, version, text))
    return modules


def npm_production_packages() -> list[tuple[str, str, str, str]]:
    frontend = ROOT / "frontend"
    raw = run(["npm", "ls", "--omit=dev", "--all", "--json"], frontend)
    tree = json.loads(raw or "{}")
    found: dict[str, tuple[str, str, str, str]] = {}

    def walk(node: dict) -> None:
        for name, dep in (node.get("dependencies") or {}).items():
            if not isinstance(dep, dict):
                continue
            version = dep.get("version") or ""
            pkg_dir = frontend / "node_modules" / name
            if pkg_dir.is_dir() and name not in found:
                license_id = ""
                manifest = pkg_dir / "package.json"
                if manifest.is_file():
                    try:
                        meta = json.loads(manifest.read_text(encoding="utf-8"))
                        lic = meta.get("license")
                        if isinstance(lic, str):
                            license_id = lic
                        elif isinstance(lic, dict):
                            license_id = lic.get("type", "")
                    except (OSError, ValueError):
                        pass
                found[name] = (name, version, license_id, read_license_texts(pkg_dir))
            walk(dep)

    walk(tree)
    return [found[key] for key in sorted(found)]


def main() -> None:
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    go_items = go_modules()
    npm_items = npm_production_packages()

    lines: list[str] = [
        "AgentDeck — THIRD-PARTY NOTICES",
        "",
        "本文件由 tools/gen_third_party_notices.py 生成，收集实际链接进桌面程序的",
        "Go 模块与随前端 bundle 分发的生产依赖的许可文本。",
        "AgentDeck 自身许可见 LICENSE；移植自 magpie 的代码见 NOTICE.md。",
        "",
        "=" * 78,
        f"Go 模块（{len(go_items)} 个）",
        "=" * 78,
        "",
    ]
    for path, version, text in go_items:
        lines += [f"### {path} {version}", "", text or "(模块内未找到许可文件)", ""]

    lines += [
        "=" * 78,
        f"前端生产依赖（{len(npm_items)} 个）",
        "=" * 78,
        "",
    ]
    for name, version, license_id, text in npm_items:
        lines += [f"### {name} {version} — {license_id or '未声明'}", "",
                  text or "(包内未找到许可文件)", ""]

    OUT.write_text("\n".join(lines), encoding="utf-8", newline="\n")
    print(f"written: {OUT} ({OUT.stat().st_size} bytes, "
          f"{len(go_items)} Go modules, {len(npm_items)} npm packages)")


if __name__ == "__main__":
    main()
