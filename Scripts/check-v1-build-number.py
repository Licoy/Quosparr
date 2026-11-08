#!/usr/bin/env python3
"""Fail closed unless the candidate exceeds every published Sparkle build."""
import argparse
import json
import pathlib
import re
import subprocess
import time
import xml.etree.ElementTree as ET

NS = "http://www.andymatuschak.org/xml-namespaces/sparkle"


def released_builds(data):
    root = ET.fromstring(data)
    values = [node.text for node in root.findall(f".//{{{NS}}}version")]
    values += [node.attrib[f"{{{NS}}}version"] for node in root.iter()
               if f"{{{NS}}}version" in node.attrib]
    if not values:
        raise ValueError("published appcast has no Sparkle build numbers")
    builds = []
    for value in values:
        if not value or not re.fullmatch(r"[0-9]+(?:\.[0-9]+)*", value):
            raise ValueError("published appcast contains an unsupported build number")
        builds.append(tuple(map(int, value.split("."))))
    return builds


def check(candidate, feeds):
    if not re.fullmatch(r"[1-9][0-9]*", candidate) or int(candidate) < 100:
        raise ValueError("candidate build must be an integer at least 100")
    builds = [build for feed in feeds for build in released_builds(feed)]
    if not builds:
        raise ValueError("no published appcasts found; cannot verify historical builds")
    if (int(candidate),) <= max(builds):
        raise ValueError("candidate build must exceed every published build")
    return max(builds)


def gh(*args, attempts=4, delay=10):
    # Runner networking to the API times out now and then; every call only
    # reads, so retrying is safe and a lasting failure still fails closed.
    for attempt in range(1, attempts + 1):
        try:
            return subprocess.run(["gh", *args], check=True, stdout=subprocess.PIPE).stdout
        except subprocess.CalledProcessError:
            if attempt == attempts:
                raise
            time.sleep(delay * attempt)


def published_feeds(repository):
    raw = gh("api", f"repos/{repository}/releases?per_page=100", "--paginate", "--slurp")
    releases = [release for page in json.loads(raw) for release in page if not release["draft"]]
    assets = [asset for release in releases for asset in release["assets"]
              if asset["name"] in ("appcast-arm64.xml", "appcast-x86_64.xml", "quosparr-appcast-arm64.xml", "quosparr-appcast-x86_64.xml")]
    return [gh("api", asset["url"], "-H", "Accept: application/octet-stream") for asset in assets]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", default="mygo.json")
    parser.add_argument("--repository", default="Licoy/Quosparr")
    parser.add_argument("--appcasts", nargs="+", help="explicit offline fixtures, never an automatic fallback")
    args = parser.parse_args()
    config = json.loads(pathlib.Path(args.config).read_text())
    candidate = str(config["macos"]["infoPlist"]["CFBundleVersion"])
    feeds = ([pathlib.Path(file).read_bytes() for file in args.appcasts]
             if args.appcasts else published_feeds(args.repository))
    maximum = check(candidate, feeds)
    print(f"Build {candidate} exceeds published maximum {'.'.join(map(str, maximum))}")


if __name__ == "__main__":
    main()
