"""Extract a tagged release's title and notes from the changelog."""

from __future__ import annotations

import os
import re
import sys
from datetime import date
from pathlib import Path


def release_notes(changelog: str, tag: str) -> tuple[str, str]:
    """Return the latest release heading and body, requiring a unique matching entry."""
    sections = re.split(r"^## ", changelog, flags=re.MULTILINE)[1:]
    sections = [section for section in sections if section.partition("\n")[0] != "Unreleased"]
    matches = [section for section in sections if section.partition("\n")[0].startswith(f"{tag} (")]
    if len(matches) != 1:
        raise ValueError(f"Expected exactly one changelog entry for {tag}, found {len(matches)}")
    if matches[0] != sections[0]:
        raise ValueError(f"Tag {tag} must match the latest changelog entry")
    title, _, body = matches[0].partition("\n")
    match = re.fullmatch(rf"{re.escape(tag)} \((\d{{4}}-\d{{2}}-\d{{2}})\)", title)
    if match is None:
        raise ValueError(f"Invalid release heading: {title}")
    date.fromisoformat(match[1])
    if not body.strip():
        raise ValueError(f"Empty release notes for {tag}")
    return title, body.strip() + "\n"


def main() -> None:
    """Validate the tag and write notes and GitHub Actions outputs."""
    tag, version = sys.argv[1:]
    if re.fullmatch(r"v\d+\.\d+\.\d+", tag) is None or tag != f"v{version}":
        raise ValueError(f"Tag {tag} does not match package version {version} or vX.Y.Z")
    title, notes = release_notes(Path("docs/changelog.md").read_text(), tag)
    Path("release-notes.md").write_text(notes)
    with Path(os.environ["GITHUB_OUTPUT"]).open("a") as output:
        output.write(f"title={title}\n")


if __name__ == "__main__":
    main()
