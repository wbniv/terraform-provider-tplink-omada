#!/usr/bin/env python3
"""Verify the vendored distribution and, optionally, the managed lab's JARs."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tarfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--controller", action="store_true", help="also compare the running lab's application JARs")
args = parser.parse_args()
root = Path(__file__).resolve().parent
manifest = json.loads((root / "manifest.json").read_text())
archive = root / manifest["filename"]
if not archive.exists() or archive.stat().st_size < 1024:
    raise SystemExit("Omada archive is absent or still an LFS pointer. Run git lfs pull.")
with archive.open("rb") as stream:
    digest = hashlib.file_digest(stream, "sha256").hexdigest()
if digest != manifest["sha256"]:
    raise SystemExit("Vendored Omada archive checksum mismatch")
result = {"version": manifest["version"], "archive_sha256": digest, "archive_bytes": archive.stat().st_size}
if args.controller:
    container = "omada-provider-lab-controller-1"
    info = json.loads(subprocess.check_output(["docker", "inspect", container]))[0]
    if info["Config"]["Labels"].get("omada.provider.lab") != "true" or info["Config"]["Image"] != manifest["controller_image"]:
        raise SystemExit("Unexpected lab container identity/image")
    expected = {}
    with tarfile.open(archive, "r:gz") as package:
        for entry in package:
            if entry.isfile() and "/lib/" in entry.name and entry.name.endswith(".jar"):
                with package.extractfile(entry) as stream:
                    expected[Path(entry.name).name] = hashlib.file_digest(stream, "sha256").hexdigest()
    raw = subprocess.check_output([
        "docker", "exec", container, "find", "/opt/tplink/EAPController/lib", "-maxdepth", "1",
        "-name", "*.jar", "-exec", "sha256sum", "{}", "+",
    ], text=True)
    actual = {Path(line.split(maxsplit=1)[1]).name: line.split(maxsplit=1)[0] for line in raw.splitlines()}
    if len(expected) != manifest["application_jars"] or expected != actual:
        raise SystemExit("Controller JARs do not exactly match the vendored distribution")
    result["matched_application_jars"] = len(expected)
print(json.dumps(result, indent=2))
