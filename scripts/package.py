#!/usr/bin/env python3
"""Build an application with embedded licenses and record its SHA-256 checksum."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
TARGETS = {
    "windows/amd64": ("windows-x64", "exe", ("amd64",)),
    "darwin/universal": ("macos-universal", "dmg", ("arm64", "amd64")),
}


def write_licenses(path, main_package, system, architectures, env):
    goroot = subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, env=env, text=True, encoding="utf-8").strip()
    notices = [("Codex Token Float", ROOT / "LICENSE"), ("Go", Path(goroot) / "LICENSE")]
    modules = set()
    for arch in architectures:
        dependencies = subprocess.check_output(
            ["go", "list", "-deps", "-f", "{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Dir}}{{end}}{{end}}", main_package],
            cwd=ROOT, env=dict(env, GOOS=system, GOARCH=arch), text=True, encoding="utf-8",
        )
        modules.update(line for line in dependencies.splitlines() if line)

    for module in sorted(modules):
        name, directory = module.split("\t", 1)
        for pattern in ("LICENSE*", "COPYING*", "NOTICE*", "PATENTS*"):
            for license_file in sorted(Path(directory).glob(pattern)):
                if license_file.is_file():
                    notices.append((name, license_file))

    sections = [f"{name} / {file.name}\n\n".encode() + file.read_bytes().rstrip() for name, file in notices]
    path.write_bytes(b"\n\n".join(sections) + b"\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", required=True, choices=TARGETS)
    args = parser.parse_args()
    config = json.loads((ROOT / "mygo.json").read_text(encoding="utf-8"))
    label, extension, architectures = TARGETS[args.target]
    system, _ = args.target.split("/")
    output = ROOT / "dist" / label
    output.mkdir(parents=True, exist_ok=True)
    path = output / f"Codex-Token-Float-{config['version']}-{label}.{extension}"
    env = dict(os.environ, CGO_ENABLED="0")
    env["GOFLAGS"] = (env.get("GOFLAGS", "") + " -buildvcs=false -mod=readonly").strip()

    with tempfile.TemporaryDirectory(prefix="codex-token-float-") as temporary:
        build_dir = Path(temporary)
        licenses = build_dir / "licenses.txt"
        write_licenses(licenses, config["main"], system, architectures, env)
        overlay = build_dir / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(ROOT / "assets" / "licenses.txt"): str(licenses)}}), encoding="utf-8")
        env["GOFLAGS"] += f' "-overlay={overlay.as_posix()}"'
        subprocess.run(
            ["go", "tool", "mygo", "build", "-platform", args.target, "-skip-notarize", "-o", temporary],
            cwd=ROOT, env=env, check=True,
        )
        built = build_dir / args.target.replace("/", "-")
        filename = f"{config['name']}.{extension}"
        if system == "darwin":
            filename = f"{config['name']} {config['version']}.{extension}"
        shutil.copyfile(built / filename, path)

    with path.open("rb") as package:
        checksum = hashlib.file_digest(package, "sha256").hexdigest()
    (output / "SHA256SUMS.txt").write_text(f"{checksum}  {path.name}\n", encoding="ascii", newline="\n")
    print(f"{checksum}  {path}")


if __name__ == "__main__":
    main()
