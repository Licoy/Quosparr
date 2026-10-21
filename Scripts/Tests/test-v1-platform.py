#!/usr/bin/env python3
import importlib.util
import io
import os
import pathlib
import struct
import subprocess
import tarfile
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("platform_package", ROOT / "Scripts/verify-v1-platform.py")
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)
sqlite_spec = importlib.util.spec_from_file_location("embedded_sqlite", ROOT / "Scripts/prepare-windows-sqlite.py")
embedded_sqlite = importlib.util.module_from_spec(sqlite_spec)
sqlite_spec.loader.exec_module(embedded_sqlite)


def tar_bytes(entries):
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w:gz") as archive:
        for name, data, mode in entries:
            info = tarfile.TarInfo(name)
            info.size, info.mode = len(data), mode
            archive.addfile(info, io.BytesIO(data))
    return out.getvalue()


def deb_bytes(control, entries):
    members = [("debian-binary", b"2.0\n"), ("control.tar.gz", tar_bytes([("control", control, 0o644)])),
               ("data.tar.gz", tar_bytes(entries))]
    out = bytearray(b"!<arch>\n")
    for name, data in members:
        out.extend(f"{name:<16}{0:<12}{0:<6}{0:<6}{'100644':<8}{len(data):<10}`\n".encode())
        out.extend(data)
        if len(data) % 2:
            out.extend(b"\n")
    return out


class PlatformPackages(unittest.TestCase):
    def linux_fixture(self, stage, failure=None):
        files = {"quosparr": b"fixture-executable", "libmygo-sqlite3.so": b"fixture-sqlite", "LICENSE": b"fixture-license"}
        for name, data in files.items():
            (stage / name).write_bytes(data)
        entries = [("opt/quosparr/" + name, data, 0o755) for name, data in files.items()]
        entries += [("usr/share/applications/quosparr.desktop", b"Name=Quosparr\nExec=/opt/quosparr/quosparr %U\n", 0o644)]
        entries += [(f"usr/share/icons/hicolor/{n}x{n}/apps/quosparr.png", b"\x89PNG\r\n\x1a\n", 0o644) for n in (256, 512)]
        arch = "arm64" if failure == "architecture" else "amd64"
        depends = "libgtk-3-0, libayatana-appindicator3-1" + (", libwebkit2gtk-4.1-0" if failure == "webkit" else "")
        control = f"Package: quosparr\nVersion: 1.0.0\nArchitecture: {arch}\nDepends: {depends}\n".encode()
        if failure == "missing sqlite":
            entries = [entry for entry in entries if not entry[0].endswith(".so")]
        elif failure == "tampered sqlite":
            entries[1] = (entries[1][0], b"tampered", entries[1][2])
        elif failure == "not executable":
            entries[0] = (entries[0][0], entries[0][1], 0o644)
        data = deb_bytes(control, entries)
        if failure == "truncated":
            data = data[:-100]
        (stage / "quosparr_1.0.0_amd64.deb").write_bytes(data)

    def test_debian_payload_and_metadata(self):
        with tempfile.TemporaryDirectory() as temporary:
            stage = pathlib.Path(temporary)
            self.linux_fixture(stage)
            package.verify_deb({"version": "1.0.0"}, stage, "amd64")

    def test_debian_rejects_missing_tampered_wrong_arch_and_truncated_payloads(self):
        for failure in ("architecture", "webkit", "missing sqlite", "tampered sqlite", "not executable", "truncated"):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                stage = pathlib.Path(temporary)
                self.linux_fixture(stage, failure)
                with self.assertRaises((ValueError, KeyError)):
                    package.verify_deb({"version": "1.0.0"}, stage, "amd64")

    def test_archive_rejects_path_escape_and_duplicates(self):
        for entries in ([('../escape', b'x', 0o644)], [('a', b'x', 0o644), ('a', b'y', 0o644)]):
            with self.assertRaises(ValueError):
                package.tar_members(tar_bytes(entries))

    def test_windows_review_archive_contains_only_runtime_payload(self):
        with tempfile.TemporaryDirectory() as temporary:
            stage = pathlib.Path(temporary)
            files = {"Quosparr.exe": b"application-with-embedded-sqlite", "LICENSE": b"license"}
            for name, data in files.items():
                (stage / name).write_bytes(data)
            (stage / "Quosparr Setup 1.0.0 amd64.exe").write_bytes(b"installer")
            (stage / "update-windows-amd64.json").write_bytes(b"must-not-archive")
            package.archive_windows({"version": "1.0.0"}, stage, "windows/amd64")
            archive = package.tar_members((stage / "quosparr-1.0.0-windows-amd64.tar.gz").read_bytes())
            self.assertEqual({name: value[0] for name, value in archive.items()}, files)

    def test_windows_cli_copy_is_private_to_build_and_removable(self):
        module = pathlib.Path(subprocess.check_output(
            ["go", "list", "-m", "-f", "{{.Dir}}", "github.com/egoist/mygo"], cwd=ROOT, text=True).strip())
        source = module / "cmd/mygo/natives.go"
        original = source.read_bytes()
        with tempfile.TemporaryDirectory() as temporary:
            stage = pathlib.Path(temporary)
            embedded_sqlite.prepare(module, stage)
            cli = stage / "mygo-cli"
            self.assertEqual(source.read_bytes(), original, "module cache must remain unchanged")
            self.assertEqual((cli / "go.mod").read_bytes(), (module / "go.mod").read_bytes())
            self.assertIn('goos == "windows" && l.Name == "mygo-sqlite3"', (cli / "cmd/mygo/natives.go").read_text())
            for directory in [cli, *(path for path in cli.rglob("*") if path.is_dir())]:
                self.assertTrue(directory.stat().st_mode & 0o200, f"build directory cannot be cleaned: {directory}")

    def test_windows_installer_must_be_pe(self):
        with tempfile.TemporaryDirectory() as temporary:
            stage = pathlib.Path(temporary)
            setup = stage / "Quosparr Setup 1.0.0 amd64.exe"
            setup.write_bytes(b"not-an-installer")
            with self.assertRaises(ValueError):
                package.verify({"version": "1.0.0"}, stage, "windows/amd64")
            data = bytearray(128)
            data[:2] = b"MZ"
            struct.pack_into("<I", data, 0x3c, 80)
            data[80:84] = b"PE\0\0"
            setup.write_bytes(data)
            package.verify({"version": "1.0.0"}, stage, "windows/amd64")
            (stage / "mygo-sqlite3.dll").write_bytes(b"external DLL")
            with self.assertRaisesRegex(ValueError, "SQLite must be embedded"):
                package.verify({"version": "1.0.0"}, stage, "windows/amd64")

    def test_packages_reject_source_only_licenses(self):
        for target in ("windows/amd64", "linux/arm64"):
            for name in ("LOBEHUB-LICENSE", "REMIXICON-LICENSE", "TOML-LICENSE"):
                with self.subTest(target=target, name=name), tempfile.TemporaryDirectory() as temporary:
                    stage = pathlib.Path(temporary)
                    (stage / name).write_text("source notice")
                    with self.assertRaisesRegex(ValueError, "source-only license"):
                        package.verify({"version": "1.0.0"}, stage, target)

    def test_production_packaging_never_falls_back_to_unsigned(self):
        env = os.environ.copy()
        env.update(V1_DEVELOPMENT_PACKAGE="0", MYGO_UPDATE_PRIVATE_KEY="", MYGO_UPDATER_PRIVATE_KEY="")
        result = subprocess.run(["bash", str(ROOT / "Scripts/package-v1-platform.sh"), "windows/amd64"], env=env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Missing MYGO_UPDATE_PRIVATE_KEY", result.stderr)
        env["V1_DEVELOPMENT_PACKAGE"] = "2"
        result = subprocess.run(["bash", str(ROOT / "Scripts/package-v1-platform.sh"), "linux/amd64"], env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)


if __name__ == "__main__":
    unittest.main()
