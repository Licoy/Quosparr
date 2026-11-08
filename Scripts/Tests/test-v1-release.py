#!/usr/bin/env python3
import importlib.util
import pathlib
import unittest
import tempfile
import subprocess
import os
import json

SCRIPT=pathlib.Path(__file__).resolve().parents[1]/"check-v1-build-number.py"
spec=importlib.util.spec_from_file_location("builds",SCRIPT)
builds=importlib.util.module_from_spec(spec)
spec.loader.exec_module(builds)

def feed(version):
 return f'<rss xmlns:sparkle="{builds.NS}"><channel><item><enclosure sparkle:version="{version}"/></item></channel></rss>'.encode()

class BuildNumberTests(unittest.TestCase):
 def test_candidate_exceeds_all_architectures(self):
  self.assertEqual(builds.check("100",[feed("87"),feed("40.1234.5")]),(87,))
 def test_historical_or_bad_build_rejected(self):
  for candidate,feeds in (("99",[feed("87")]),("100",[feed("100")]),("101",[feed("102")]),("1.0.0",[feed("87")]),("100",[]),("100",[feed("unknown")])):
   with self.assertRaises(ValueError): builds.check(candidate,feeds)

 def test_gh_retries_then_fails_closed(self):
  calls=[]
  def run(args,**kwargs):
   calls.append(args)
   if len(calls)<3: raise subprocess.CalledProcessError(1,args)
   return subprocess.CompletedProcess(args,0,stdout=b"ok")
  original,builds.subprocess.run=builds.subprocess.run,run
  try:
   self.assertEqual(builds.gh("api","x",delay=0),b"ok")
   calls.clear()
   def fail(args,**kwargs):
    calls.append(args); raise subprocess.CalledProcessError(1,args)
   builds.subprocess.run=fail
   with self.assertRaises(subprocess.CalledProcessError): builds.gh("api","x",attempts=2,delay=0)
   self.assertEqual(len(calls),2)
  finally:
   builds.subprocess.run=original

class StageConfigTests(unittest.TestCase):
 def test_stable_uses_quosparr_repository(self):
  root=pathlib.Path(__file__).resolve().parents[2]
  with tempfile.TemporaryDirectory(prefix="quosparr-config-test-") as temporary:
   env=os.environ.copy()
   env.update(V1_CONFIG_PATH=str(root/"mygo.json"),V1_UPDATE_CHANNEL="stable",V1_DEVELOPMENT_PACKAGE="0")
   subprocess.run(["python3",str(root/"Scripts/stage-v1-config.py"),str(root),temporary],env=env,check=True)
   staged=json.loads((pathlib.Path(temporary)/"mygo.json").read_text())
   self.assertEqual(staged["updates"]["github"],"Licoy/Quosparr")
   self.assertEqual(staged["updates"]["tagPrefix"],"v")
   self.assertNotIn("url",staged["updates"])

 def test_beta_uses_fixed_prerelease_feed_and_dev_disables_updates(self):
  root=pathlib.Path(__file__).resolve().parents[2]
  config=json.loads((root/"mygo.json").read_text())
  config["version"]="1.0.0-beta.1"
  with tempfile.TemporaryDirectory(prefix="runway-config-test-") as temporary:
   directory=pathlib.Path(temporary)
   source=directory/"source.json"
   source.write_text(json.dumps(config))
   env=os.environ.copy()
   env.update(V1_CONFIG_PATH=str(source),V1_UPDATE_CHANNEL="beta",V1_DEVELOPMENT_PACKAGE="0")
   stage=root/"Scripts/stage-v1-config.py"
   subprocess.run(["python3",str(stage),str(root),str(directory/"beta")],env=env,check=True)
   beta=json.loads((directory/"beta/mygo.json").read_text())
   self.assertEqual(beta["updates"]["url"],"https://github.com/Licoy/Quosparr/releases/download/v1-beta")
   self.assertNotIn("github",beta["updates"])
   self.assertNotIn("tagPrefix",beta["updates"])
   env["V1_DEVELOPMENT_PACKAGE"]="1"
   subprocess.run(["python3",str(stage),str(root),str(directory/"dev")],env=env,check=True)
   self.assertNotIn("updates",json.loads((directory/"dev/mygo.json").read_text()))

class RenameNoticeTests(unittest.TestCase):
 def test_old_feed_cannot_install_new_bundle_identity(self):
  root=pathlib.Path(__file__).resolve().parents[2]
  with tempfile.TemporaryDirectory() as temporary:
   notice=pathlib.Path(temporary)/"appcast.xml"
   subprocess.run(["python3",str(root/"Scripts/legacy-install-notice.py"),str(root/"mygo.json"),str(notice),"v1.0.0"],check=True)
   item=builds.ET.parse(notice).find(".//item")
   self.assertIsNotNone(item)
   self.assertIsNone(item.find("enclosure"))
   self.assertEqual(item.findtext("link"),"https://github.com/Licoy/Quosparr/releases/tag/v1.0.0")
   self.assertEqual(item.findtext(f"{{{builds.NS}}}version"),str(json.loads((root/"mygo.json").read_text())["macos"]["infoPlist"]["CFBundleVersion"]))

if __name__=="__main__": unittest.main()
