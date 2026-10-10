"""Reserve the next release version without bumping again after a failed build."""
import json
import os
import re
import subprocess
from pathlib import Path


def version_tuple(value):
    if not re.fullmatch(r"\d+\.\d+\.\d+", value):
        raise ValueError("Version must have the form major.minor.patch")
    v = tuple(map(int, value.split(".")))
    if v[1] >= 1000 or v[2] >= 1000:
        raise ValueError("Minor/patch must be below 1000 for the Android version code")
    code = v[0] * 1_000_000 + v[1] * 1000 + v[2]
    if not 0 < code <= 2_100_000_000:
        raise ValueError("Android version code is outside its supported range")
    return v


def next_version(base, tags, requested=""):
    floor = version_tuple(base)
    versions = [version_tuple(t[1:]) for t in tags if re.fullmatch(r"v\d+\.\d+\.\d+", t)]
    latest = max(versions) if versions else None
    if requested:
        candidate = version_tuple(requested)
        if candidate < floor or (latest is not None and candidate <= latest):
            raise ValueError("The requested version must be newer than existing releases")
    elif latest is not None and latest >= floor:
        major, minor, patch = latest
        if patch == 999:
            minor, patch = minor + 1, 0
        else:
            patch += 1
        if minor == 1000:
            major, minor = major + 1, 0
        candidate = (major, minor, patch)
    else:
        candidate = floor
    text = ".".join(map(str, candidate))
    version_tuple(text)
    return text, candidate[0] * 1_000_000 + candidate[1] * 1000 + candidate[2]


def main():
    root = Path(__file__).resolve().parents[1]
    tags = subprocess.check_output(["git", "tag", "--list", "v*"], text=True).splitlines()
    version, code = next_version((root / "VERSION").read_text().strip(), tags, os.getenv("REQUESTED_VERSION", "").strip())
    (root / "VERSION").write_text(version + "\n")
    toml = root / "cmd/scaner/FyneApp.toml"
    text = re.sub(r'^Version = ".*"$', f'Version = "{version}"', toml.read_text(), flags=re.M)
    text = re.sub(r"^Build = \d+$", f"Build = {code}", text, flags=re.M)
    toml.write_text(text)
    manifest = root / "packaging/windows/app.manifest"
    text = re.sub(r'assemblyIdentity version="[^"]+"', f'assemblyIdentity version="{version}.0"', manifest.read_text())
    manifest.write_text(text)
    status = {
        "state": "building", "version": version,
        "run_url": f'{os.getenv("GITHUB_SERVER_URL","https://github.com")}/{os.getenv("GITHUB_REPOSITORY","salehsayyadi/scaner")}/actions/runs/{os.getenv("GITHUB_RUN_ID","")}',
        "source_sha": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
    }
    (root / ".github/scaner-build-status.json").write_text(json.dumps(status, indent=2) + "\n")
    if output := os.getenv("GITHUB_OUTPUT"):
        with open(output, "a") as f:
            f.write(f"version={version}\nversion_code={code}\n")
    print(f"Reserved version {version}, Android version code {code}")


if __name__ == "__main__":
    main()