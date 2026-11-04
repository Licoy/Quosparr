#!/usr/bin/env python3
"""Stage the existing mygo config without embedding signing credentials."""
import json
import os
import pathlib
import re
import sys

root, destination = map(pathlib.Path, sys.argv[1:3])
source = pathlib.Path(os.environ.get("V1_CONFIG_PATH", root / "mygo.json"))
config = json.loads(source.read_text())
if not re.fullmatch(r"[1-9][0-9]*", str(config["macos"]["infoPlist"]["CFBundleVersion"])):
    raise SystemExit("CFBundleVersion must be an integer")
if int(config["macos"]["infoPlist"]["CFBundleVersion"]) < 100:
    raise SystemExit("CFBundleVersion must be at least 100")
config["main"] = str(root)
config["icon"] = str(root / config["icon"])
config["resources"] = [str(root / entry) for entry in config.get("resources", [])]
if os.environ.get("V1_DEVELOPMENT_PACKAGE", "0") == "1":
    config.pop("updates", None)
elif os.environ.get("V1_UPDATE_CHANNEL", "stable") == "beta":
    if "-" not in config["version"]:
        raise SystemExit("beta channel requires a prerelease version in mygo.json")
    config["updates"].pop("github", None)
    config["updates"].pop("tagPrefix", None)
    config["updates"]["url"] = "https://github.com/Licoy/Quosparr/releases/download/v1-beta"
elif os.environ.get("V1_UPDATE_CHANNEL", "stable") != "stable":
    raise SystemExit("V1_UPDATE_CHANNEL must be stable or beta")
else:
    # The app picks its update feed from the version (internal/app updateFeed)
    if "-" in config["version"]:
        raise SystemExit("stable channel requires a release version in mygo.json")
    config["updates"]["tagPrefix"] = "v"
destination.mkdir(parents=True, exist_ok=True)
(destination / "mygo.json").write_text(json.dumps(config, indent=2) + "\n")
