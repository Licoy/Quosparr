#!/usr/bin/env python3
"""Verify installer payloads without executing foreign-platform binaries."""
import hashlib
import io
import json
import pathlib
import struct
import sys
import tarfile


def digest(data):
    return hashlib.sha256(data).digest()


def payload_files(stage):
    return [p for p in stage.iterdir() if not (
        p.name.startswith("Quosparr Setup ") or p.name.startswith("update-")
        or p.name.endswith((".tar.gz", ".deb")) or p.name == "install.sh")]


def archive_windows(config, stage, target):
    archive = stage / f"quosparr-{config['version']}-{target.replace('/', '-')}.tar.gz"
    with tarfile.open(archive, "w:gz") as output:
        for source in sorted(payload_files(stage)):
            output.add(source, arcname=source.name)


def ar_members(path):
    """Debian uses three ordinary ar members, with even-byte padding."""
    data = path.read_bytes()
    if data[:8] != b"!<arch>\n":
        raise ValueError("invalid Debian archive header")
    members, offset = {}, 8
    while offset < len(data):
        header = data[offset:offset + 60]
        if len(header) != 60 or header[58:] != b"`\n":
            raise ValueError("truncated Debian archive member")
        name = header[:16].decode("ascii").strip().rstrip("/")
        size = int(header[48:58].strip())
        offset += 60
        if size < 0 or offset + size > len(data) or name in members:
            raise ValueError("invalid or duplicate Debian archive member")
        members[name] = data[offset:offset + size]
        offset += size + size % 2
    if offset != len(data) or set(members) != {"debian-binary", "control.tar.gz", "data.tar.gz"}:
        raise ValueError("unexpected Debian archive structure")
    if members["debian-binary"] != b"2.0\n":
        raise ValueError("unsupported Debian archive version")
    return members


def tar_members(data):
    out = {}
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        for member in archive:
            name = member.name.removeprefix("./").rstrip("/")
            path = pathlib.PurePosixPath(name)
            if path.is_absolute() or ".." in path.parts or "\\" in name or name in out:
                raise ValueError("unsafe or duplicate package path")
            if member.isdir():
                continue
            if not member.isfile() or member.size > 256 << 20:
                raise ValueError("unexpected package entry")
            out[name] = (archive.extractfile(member).read(), member.mode)
    return out


def verify_deb(config, stage, arch):
    version = config["version"].removeprefix("v")
    if "-" in version:
        major, suffix = version.split("-", 1)
        version = major + "~" + suffix.replace("-", ".")
    version = version.replace("+", "~")
    members = ar_members(stage / f"quosparr_{version}_{arch}.deb")
    control = tar_members(members["control.tar.gz"])["control"][0].decode()
    fields = dict(line.split(": ", 1) for line in control.splitlines() if ": " in line and not line.startswith(" "))
    for key, expected in {"Package": "quosparr", "Version": version, "Architecture": arch}.items():
        if fields.get(key) != expected:
            raise ValueError(f"Debian {key} does not match target")
    for dependency in ("libgtk-3-0", "libayatana-appindicator3-1"):
        if dependency not in fields.get("Depends", ""):
            raise ValueError(f"missing runtime dependency: {dependency}")
    # Native UI only: linux.webview false keeps WebKitGTK out of the package
    if "webkit" in fields.get("Depends", "").lower():
        raise ValueError("Debian package must not depend on WebKitGTK")
    installed = tar_members(members["data.tar.gz"])
    # All runtime resources, including SQLite, must be identical
    # to the archive whose executable architecture is checked by release-assets.
    for source in payload_files(stage):
        if source.suffix in {".desktop", ".png", ".xml"}:
            continue
        for path in ([source] if source.is_file() else source.rglob("*")):
            if not path.is_file():
                continue
            name = "opt/quosparr/" + path.relative_to(stage).as_posix()
            data, mode = installed[name]
            if digest(data) != digest(path.read_bytes()):
                raise ValueError(f"installer/archive payload mismatch: {name}")
            if path.name in {"quosparr", "libmygo-sqlite3.so"} and not mode & 0o111:
                raise ValueError(f"runtime is not executable: {name}")
    desktop = installed["usr/share/applications/quosparr.desktop"][0].decode()
    if "Exec=/opt/quosparr/quosparr" not in desktop or "Name=Quosparr\n" not in desktop:
        raise ValueError("invalid installed desktop entry")
    for size in (256, 512):
        name = f"usr/share/icons/hicolor/{size}x{size}/apps/quosparr.png"
        if not installed[name][0].startswith(b"\x89PNG\r\n\x1a\n"):
            raise ValueError("missing installed PNG icon")


def verify(config, stage, target):
    extra_licenses = {"LOBEHUB-LICENSE", "REMIXICON-LICENSE", "TOML-LICENSE"}
    if any(path.name in extra_licenses for path in stage.rglob("*")):
        raise ValueError("source-only license files must not be copied into the package")
    system, arch = target.split("/")
    if system == "linux":
        verify_deb(config, stage, arch)
    else:
        if (stage / "mygo-sqlite3.dll").exists():
            raise ValueError("Windows SQLite must be embedded, not shipped beside the executable")
        setup = stage / f"Quosparr Setup {config['version']} {arch}.exe"
        data = setup.read_bytes()
        if len(data) < 64 or data[:2] != b"MZ":
            raise ValueError("missing Windows PE installer")
        offset = struct.unpack_from("<I", data, 0x3C)[0]
        if data[offset:offset + 4] != b"PE\0\0":
            raise ValueError("invalid Windows PE installer")
    print(f"Installer structure verified: {target}")


def main():
    command, config_path, directory, target = sys.argv[1:]
    if target not in {"windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64"}:
        raise ValueError("unsupported platform target")
    config, stage = json.loads(pathlib.Path(config_path).read_text()), pathlib.Path(directory)
    if command == "archive-windows" and target.startswith("windows/"):
        archive_windows(config, stage, target)
    elif command == "verify":
        verify(config, stage, target)
    else:
        raise ValueError("invalid platform verifier command")


if __name__ == "__main__":
    main()
