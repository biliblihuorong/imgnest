#!/usr/bin/env python3
"""Set the release version and update the docs that name it.

    python3 scripts/bump-version.py 0.0.9   # writes VERSION and the docs
    python3 scripts/bump-version.py --check # fails if a doc misses VERSION

Merging a change to VERSION into main publishes vX.Y.Z: release.yml creates
the tag and the GitHub Release once the image for that commit is pushed.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
VERSION_FILE = ROOT / "VERSION"
SEMVER = re.compile(r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?$")
# Docs that tell users which image tag to pull or which tag to check out.
DOC_FILES = [
    "README.md",
    "website/docs/guide/introduction.md",
    "website/docs/guide/getting-started.md",
    "website/docs/guide/deployment.md",
    "website/docs/en/guide/introduction.md",
    "website/docs/en/guide/getting-started.md",
    "website/docs/en/guide/deployment.md",
]


def current():
    version = VERSION_FILE.read_text().strip()
    if not SEMVER.match(version):
        sys.exit(f"VERSION is not a semantic version: {version!r}")
    return version


def check():
    version = current()
    missing = [name for name in DOC_FILES if version not in (ROOT / name).read_text()]
    if missing:
        sys.exit(f"these docs do not mention {version}; run scripts/bump-version.py: "
                 + ", ".join(missing))
    print(f"docs mention {version}")


def replace_version(text, old, new):
    """Replaces old as a whole version; 10.0.0.5 or 0.0.50 stay as they are."""
    pattern = re.compile(r"(?<![\d.])" + re.escape(old) + r"(?!\d|\.\d)")
    return pattern.subn(new, text)


def bump(new):
    if not SEMVER.match(new):
        sys.exit(f"not a semantic version: {new!r}")
    old = current()
    for name in DOC_FILES:
        path = ROOT / name
        text, count = replace_version(path.read_text(), old, new)
        if count:
            path.write_text(text)
        print(f"{name}: {count}")
    VERSION_FILE.write_text(new + "\n")
    print(f"VERSION: {old} -> {new}")


def main():
    args = sys.argv[1:]
    if args == ["--check"]:
        check()
    elif len(args) == 1 and not args[0].startswith("-"):
        bump(args[0].removeprefix("v"))
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
