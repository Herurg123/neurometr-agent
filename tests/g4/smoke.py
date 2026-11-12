"""G4.1 private CI smoke: tests ONLY the unpacked Windows ZIP.
No raw JSON, tokens, hardware names or private paths are logged or uploaded.
CI tooling (Go/Python) validates results, but the packaged user EXE starts
with a scrubbed PATH and without any SDK, -root or separate llmfit install.
"""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import uuid
import zipfile

ROOT=Path(__file__).resolve().parents[2]
TEST=ROOT/"tests"/"localui"/"smoke.py"
spec=importlib.util.spec_from_file_location("previous_accepted_a34_smoke",TEST)
A34=importlib.util.module_from_spec(spec)
spec.loader.exec_module(A34)
EXPECTED_CATALOG="1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2"

def fail(condition,message):
    if not condition:raise AssertionError(message)

def digest(path):
    h=hashlib.sha256()
    with open(path,"rb") as fh:
        for b in iter(lambda:fh.read(1024*1024),b""):h.update(b)
    return h.hexdigest()

def launch(exe, working_directory):
    environment=dict(os.environ)
    # The end-user binary does not get a Go/Python/Node/llmfit executable
    # through PATH; external SDKs installed on CI runner cannot mask errors.
    environment["PATH"]=str(Path(os.environ["SYSTEMROOT"])/"System32")
    for key in ("GOROOT","GOPATH","GOFLAGS","NODE_PATH","PYTHONPATH","NEUROMETR_A3_TEST_ONLY","NEUROMETR_CATALOG_PATH"):
        environment.pop(key,None)
    proc=subprocess.Popen([str(exe),"-no-browser"],cwd=working_directory,
        env=environment,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,
        text=True,bufsize=1)
    first=proc.stdout.readline().strip()
    if not first.startswith("NEUROMETR_LOCAL_UI=http://127.0.0.1:"):
        A34.stop(proc)
        raise AssertionError("packaged EXE failed its relative-root/no-SDK startup")
    url=first.split("=",1)[1]
    fail("?" not in url,"packaged URL contains query string")
    return proc,url

def run_report(exe,workdir,prior=None):
    proc,url=launch(exe,workdir)
    try:
        opener,_=A34.browser(url)
        csrf=A34.session(opener,url)
        A34.local_external_dial_denied(url)
        origin=url.rstrip("/")
        a,_,_=A34.request(opener,url+"api/history",headers={"Origin":"http://untrusted.invalid"})
        fail(a==403,"foreign origin accepted")
        a,_,_=A34.request(opener,url+"api/run",method="POST",data=b"{}",
            headers={"Content-Type":"application/json","Origin":origin})
        fail(a==403,"CSRF-less mutation accepted")
        a,_,_=A34.request(opener,url+"api/run",method="GET")
        fail(a not in (200,201),"GET launched new diagnostic")
        if prior is not None:
            status,b,_=A34.request(opener,url+"api/history")
            fail(status==200,"history unavailable after packaged restart")
            items=json.loads(b)["entries"]
            fail(any(x["id"]==prior[0] for x in items),"stored report disappeared after restart")
            status,old,_=A34.request(opener,url+"api/report/"+prior[0])
            fail(status==200 and old==prior[1],"stored bytes changed after restart")
        status,created,_=A34.request(opener,url+"api/run",method="POST",data=b"{}",
            headers={"Content-Type":"application/json","Origin":origin,"X-Neurometr-CSRF":csrf})
        fail(status==201,"packaged native hardware diagnostic failed")
        runid=json.loads(created)["id"]
        uuid.UUID(runid)
        status,raw,_=A34.request(opener,url+"api/report/"+runid)
        fail(status==200 and len(raw)<=2*1024*1024,"saved JSON missing, invalid size")
        report=A34.python_oracle(raw)
        A34.go_validate(raw)
        fail(report["catalog_ref"]["origin"]=="bundled","non-bundled catalog")
        fail(report["catalog_ref"]["version"]=="2026.11.11.2","unexpected catalog version")
        fail(report["catalog_ref"]["sha256"]==EXPECTED_CATALOG,"unexpected catalog SHA")
        fail(report["hardware"]["os"]["family"]=="windows","not native Windows collector")
        fail(report["hardware"]["cpu"]["logical_processors"] is not None,"CPU missing")
        fail(report["hardware"]["ram"]["physical_total_bytes"] is not None,"RAM missing")
        fail(report["model_assessments"]==[] and report["runtime_checks"]==[] and report["cloud_checks"]==[],"fake model/cloud results")
        fail(report["engines"]==[],"optional llmfit executed in fast route")
        for item in (b'GITHUB_TOKEN',b'USERPROFILE',b'"hostname"',b'"username"',b'"serial_number"',b'"mac_address"',b'"ip_address"',b'private_key'):
            fail(item not in raw,"PII/security marker leaked")
        status,download,hdr=A34.request(opener,url+"api/export/"+runid)
        fail(status==200 and download==raw and "attachment" in hdr.get("Content-Disposition",""),"manual JSON export broken")
        return runid,raw,len(report["hardware"]["gpus"])
    finally:
        A34.stop(proc)

def integrity_checks(package,archive,official):
    manifest_path=package/"release-manifest.json"
    manifest=json.loads(manifest_path.read_text(encoding="utf-8"))
    fail(manifest["format"]=="neurometr.g4.1.internal-inventory","unexpected manifest format")
    fail(manifest["distribution"]=="internal-ci-only-not-for-publication","manifest wrongly declares a released package")
    fail(manifest["llmfit_version"]=="1.1.20","wrong llmfit version")
    fail(manifest["catalog_version"]=="2026.11.11.2","unexpected catalog")
    fail(manifest["catalog_sha256"]==EXPECTED_CATALOG,"catalog not pinned")
    fail(manifest["llmfit_official_archive_sha256"]=="1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489","source archive pin changed")
    fail(manifest["agent_source_commit"]==os.environ["G4_EXPECTED_SOURCE_COMMIT"],"manifest source commit wrong")
    files=manifest["files"]
    fail(files==sorted(files,key=lambda x:x["path"]),"manifest file order unstable")
    names=[entry["path"] for entry in files]
    fail(len(names)==len(set(x.lower() for x in names)),"casefold duplicate in manifest")
    present=set()
    for entry in files:
        p=package.joinpath(*entry["path"].split("/"))
        fail(p.is_file() and not p.is_symlink(),"manifest file missing or unsafe")
        fail(p.stat().st_size==entry["bytes"],"file size mismatch")
        fail(digest(p)==entry["sha256"],"file SHA-256 mismatch")
        present.add(entry["path"])
    physical={p.relative_to(package).as_posix() for p in package.rglob("*") if p.is_file()}
    fail(physical==present|{"release-manifest.json"},"package contains unlisted file")
    fail(digest(package/"bin"/"llmfit.exe")==digest(official),"bundled llmfit binary not identical to Authenticode-checked source")
    fail(digest(package/"bin"/"llmfit.exe")==manifest["llmfit_binary_sha256"],"llmfit PE hash mismatch")
    fail(digest(package/"bundled"/"catalog.json")==EXPECTED_CATALOG,"catalog content mismatch")
    fail("Neurometr.exe" in present and "README_RU.txt" in present,"missing runnable offline application")
    fail(all(name in present for name in (
        "LICENSES/llmfit-MIT.txt","LICENSES/jsonschema-Apache-2.0.txt",
        "LICENSES/golang-x-text-BSD-3-Clause.txt","LICENSES/go-toolchain-BSD-3-Clause.txt",
        "THIRD_PARTY_NOTICES_RU.txt")),"license or attribution missing")
    fail("MIT License" in (package/"LICENSES"/"llmfit-MIT.txt").read_text(encoding="utf-8"),"llmfit MIT text absent")
    print(f"G4_1_MANIFEST=PASS INVENTORY_FILES={len(files)} LLMFIT_BINARY_HASH=PASS OFFICIAL_SOURCE=PASS")
    # Negative ZIP checks use an in-memory test copy in RUNNER_TEMP, never
    # rewrite the approved package or publish mutated ZIPs.
    with zipfile.ZipFile(archive) as z:
        names=z.namelist()
        fail(not any(".." in n or "\\" in n for n in names),"unsafe release ZIP path")
        fail(len(names)==len(set(n.casefold() for n in names)),"duplicate ZIP entry")
    print("G4_1_ZIP_PATH_TRAVERSAL=DENIED DUPLICATES=DENIED")

def negative_zip_integrity(archive):
    # Mutate a verified FULL package and assert no extraction starts. This
    # exercises manifest, missing file, duplicate-case aliases, symlinks and
    # path traversal beyond the synthetic Go unit tests.
    with zipfile.ZipFile(archive) as original:
        entries=[(x,original.read(x.filename)) for x in original.infolist()]
    root="Neurometr-Windows-x64/"
    def construct_bad(kind,target):
        with zipfile.ZipFile(target,"w") as out:
            for info,raw in entries:
                if kind=="missing_file" and info.filename==root+"README_RU.txt":
                    continue
                if kind=="file_digest" and info.filename==root+"README_RU.txt":
                    raw=raw+b"\nMUTATED\n"
                if kind=="manifest_metadata" and info.filename==root+"release-manifest.json":
                    raw=raw.replace(b"internal-ci-only-not-for-publication",b"public-release",1)
                out.writestr(info,raw)
            if kind=="traversal":
                out.writestr(root+"../outside-private-check.txt",b"would be unsafe")
            elif kind=="duplicate_case":
                out.writestr(root+"BIN/LLMFIT.EXE",b"MZalias")
            elif kind=="symlink":
                import stat
                link=zipfile.ZipInfo(root+"symlink-link")
                link.create_system=3
                link.external_attr=(stat.S_IFLNK | 0o777)<<16
                out.writestr(link,b"../../untrusted")
    with tempfile.TemporaryDirectory(prefix="g4-hostile-zip-private-") as temporary:
        base=Path(temporary)
        for kind in ("missing_file","file_digest","manifest_metadata","traversal","duplicate_case","symlink"):
            modified=base/(kind+".zip")
            extract=base/(kind+"-should-not-exist")
            construct_bad(kind,modified)
            p=subprocess.run(["go","run","./cmd/g4pack","verify","-zip",str(modified),
                "-dest",str(extract)],cwd=ROOT,stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,timeout=45)
            fail(p.returncode!=0, "hostile ZIP incorrectly accepted: "+kind)
            fail(not extract.exists(),"hostile ZIP wrote outside accepted extraction: "+kind)
            print("G4_1_NEGATIVE_ZIP="+kind+" REJECTED=PASS")


def main():
    fail(sys.platform=="win32","Windows-only G4.1 smoke")
    fail(len(sys.argv)==4,"need unpacked private folder, original private ZIP and Authenticode-checked CLI")
    package=Path(sys.argv[1])
    archive=Path(sys.argv[2])
    official=Path(sys.argv[3])
    fail(" " in str(package),"unpacked path must contain spaces")
    integrity_checks(package,archive,official)
    negative_zip_integrity(archive)
    workdir=tempfile.TemporaryDirectory(prefix="G4 CWD outside package ")
    try:
        work=Path(workdir.name)
        first=run_report(package/"Neurometr.exe",work)
        print(f"G4_1_PACKAGED_NATIVE=PASS FULL_GO=PASS WEB_PYTHON=PASS JSON_BYTES={len(first[1])} GPU_COUNT={first[2]}")
        second=run_report(package/"Neurometr.exe",work,prior=first)
        print("G4_1_UNPACKED_RESTART_HISTORY=PASS JSON_EXPORT=PASS HOST_CSRF=DENIED NO_SDK_PATH=PASS")
        # Approved zip remains immutable; only the disposable extraction is
        # mutated to prove the optional CLI is not a fast-path dependency.
        cli=package/"bin"/"llmfit.exe"
        backup=package/"bin"/"llmfit.exe.backup"
        cli.rename(backup)
        try:
            run_report(package/"Neurometr.exe",work,prior=second)
            cli.write_bytes(b"MZinvalid-binary-do-not-execute")
            run_report(package/"Neurometr.exe",work,prior=second)
        finally:
            cli.unlink(missing_ok=True)
            backup.rename(cli)
        print("G4_1_OPTIONAL_LLMFIT_MISSING=PASS CORRUPT=PASS NO_AUTO_CLI=PASS")
    finally:
        workdir.cleanup()

if __name__=="__main__":
    main()
