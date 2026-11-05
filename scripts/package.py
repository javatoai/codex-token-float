#!/usr/bin/env python3
"""Build a portable MyGO package and its SHA-256 checksum."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile


ROOT = Path(__file__).resolve().parents[1]
TARGETS = {
    "windows/amd64": "windows-x64",
    "darwin/arm64": "macos-arm64",
    "darwin/amd64": "macos-x64",
}


def add_licenses(archive, main_package, target, env):
    system, arch = target.split("/")
    modules = subprocess.check_output(
        ["go", "list", "-deps", "-f", "{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Dir}}{{end}}{{end}}", main_package],
        cwd=ROOT, env=dict(env, GOOS=system, GOARCH=arch), text=True, encoding="utf-8",
    )
    for module in sorted(set(modules.splitlines()) - {""}):
        name, directory = module.split("\t", 1)
        for pattern in ("LICENSE*", "COPYING*", "NOTICE*"):
            for license_file in sorted(Path(directory).glob(pattern)):
                if license_file.is_file():
                    archive.write(license_file, f"licenses/{name.replace('/', '_')}-{license_file.name}")

    goroot = subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, env=env, text=True, encoding="utf-8").strip()
    archive.write(Path(goroot) / "LICENSE", "licenses/Go-LICENSE")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", required=True, choices=TARGETS)
    args = parser.parse_args()
    config = json.loads((ROOT / "mygo.json").read_text(encoding="utf-8"))
    label = TARGETS[args.target]
    output = ROOT / "dist" / label
    output.mkdir(parents=True, exist_ok=True)
    path = output / f"Codex-Token-Float-{config['version']}-{label}.zip"
    env = dict(os.environ, CGO_ENABLED="0")
    env["GOFLAGS"] = (env.get("GOFLAGS", "") + " -buildvcs=false -mod=readonly").strip()

    with tempfile.TemporaryDirectory(prefix="codex-token-float-") as temporary:
        subprocess.run(
            ["go", "tool", "mygo", "build", "-platform", args.target, "-skip-dmg", "-skip-notarize", "-o", temporary],
            cwd=ROOT, env=env, check=True,
        )
        built = Path(temporary) / args.target.replace("/", "-")
        with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
            if args.target.startswith("windows/"):
                archive.write(built / (config["name"] + ".exe"), "codex-token-float.exe")
            else:
                bundle = built / (config["name"] + ".app")
                executable = bundle / "Contents" / "MacOS" / config["name"]
                if not executable.is_file():
                    raise FileNotFoundError(executable)
                for file in sorted(bundle.rglob("*")):
                    if file.is_file():
                        archive.write(file, file.relative_to(built))
            for name in ("README.md", "LICENSE"):
                archive.write(ROOT / name, name)
            add_licenses(archive, config["main"], args.target, env)

    with path.open("rb") as package:
        checksum = hashlib.file_digest(package, "sha256").hexdigest()
    (output / "SHA256SUMS.txt").write_text(f"{checksum}  {path.name}\n", encoding="ascii", newline="\n")
    print(f"{checksum}  {path}")


if __name__ == "__main__":
    main()
