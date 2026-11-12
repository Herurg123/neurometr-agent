"""Reconstruct source dependency scope for pinned Windows llmfit CLI (not EXE attestation).
Run only in Windows GitHub CI, against pinned upstream commit and Cargo.lock.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

COMMIT = "bb5771394e88e352c266f4bf8bdb7a5295a998a8"
TARGET = "x86_64-pc-windows-msvc"
NPM = {"js-tokens":"4.0.0", "loose-envify":"1.4.0", "react":"18.3.1",
       "react-dom":"18.3.1", "scheduler":"0.23.2"}

def command(args, cwd):
    return subprocess.run(args, cwd=cwd, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, text=True, encoding="utf-8",
                          errors="replace", timeout=420, check=True).stdout

def tree_packages(text):
    packages = set()
    for line in text.splitlines():
        if not line.strip() or line.strip().startswith("["):
            continue
        m = re.fullmatch(r"([A-Za-z0-9_-]+) v(\d+\.\d+\.\d+(?:[-+][^\s]+)?)(?: \(.+\))?", line.strip())
        if not m:
            raise ValueError("unrecognized cargo tree line: " + line[:120])
        packages.add((m.group(1),m.group(2)))
    return packages

def collect(meta, tree, npm_lock):
    names = tree_packages(tree)
    metadata = {(p["name"],p["version"]):p for p in meta["packages"]}
    if not names or names - metadata.keys():
        raise ValueError("cargo tree/metadata disagree")
    if not {"llmfit","llmfit-core"} <= {name for name,_ in names}:
        raise ValueError("expected CLI/core roots absent")
    if {"llmfit-desktop","async-nats","tauri"} & {name for name,_ in names}:
        raise ValueError("desktop or optional components leaked into CLI scope")
    records=[]
    for name,version in sorted(names):
        p=metadata[(name,version)]
        directory=Path(p["manifest_path"]).parent
        notices=sorted({x.name for x in directory.iterdir() if x.is_file() and
            re.fullmatch(r"(LICENSE|LICENCE|COPYING|NOTICE|COPYRIGHT|AUTHORS)([-._].*)?",x.name,re.I)})
        records.append({"name":name,"version":version,"license_spdx":p.get("license"),
                        "license_file":p.get("license_file"),
                        "notice_files":notices,"source":p.get("source"),
                        "repository":p.get("repository")})
    production={key.removeprefix("node_modules/"):v
                for key,v in npm_lock["packages"].items()
                if key.startswith("node_modules/") and not v.get("dev")}
    if {k:v["version"] for k,v in production.items()} != NPM:
        raise ValueError("pinned npm production dependencies changed")
    if any(v.get("license")!="MIT" for v in production.values()):
        raise ValueError("npm license mismatch")
    return {
      "schema":"neurometr.g4.2.llmfit-cli-target-audit.v1",
      "upstream_tag":"v1.1.20","upstream_commit":COMMIT,
      "target":TARGET,"root_package":"llmfit","feature_scope":"default",
      "cargo_edges":"normal,build", "dev_dependencies_excluded":True,
      "other_workspace_members_excluded":True,
      "rust_packages":records,
      "npm_production_packages":[{"name":k,"version":v["version"],"license":v["license"]}
                                 for k,v in sorted(production.items())],
      "rust_license_spdx_unresolved":[p["name"]+"@"+p["version"] for p in records if not p["license_spdx"]],
      "rust_crate_notice_files_absent":[p["name"]+"@"+p["version"] for p in records if not p["notice_files"]],
      "full_rust_license_texts_in_package":False,
      "official_exe_compiled_graph_attested":False,
      "official_exe_embedded_web_assets_attested":False,
      "status":"candidate_graph_audit_not_redistribution_clearance"
    }


def generate_license_bundle(meta, report, output):
    """Preserve verbatim LICENSE/NOTICE-like files from locked Cargo source caches.

    This includes build-only crates conservatively. Missing vendor files stay
    explicitly missing, not silently replaced by generic SPDX prose.
    """
    lookup = {(p["name"],p["version"]):p for p in meta["packages"]}
    sections = [
        "НЕЙРОМЕТР: оригинальные лицензионные файлы Rust crates (SOURCE RECONSTRUCTION)",
        "Target: " + TARGET,
        "Source commit: " + COMMIT,
        "Scope: llmfit CLI default, cargo normal+build, target-filtered; no desktop/dev.",
        "Это исходная инвентаризация, НЕ аттестация бинарника от издателя.",
        "MIT и Apache-2.0 общих проектов также см. в отдельных файлах LICENSES/.",
        "Лицензии отсутствующих исходных файлов НЕ реконструированы из SPDX.",
        "BEGIN_LICENSE_TEXTS",
    ]
    missing=[]
    count=0
    for rec in report["rust_packages"]:
        key=(rec["name"],rec["version"])
        package=lookup[key]
        directory=Path(package["manifest_path"]).parent
        section_title=f"{rec['name']}@{rec['version']}"
        for filename in rec["notice_files"]:
            source=directory/filename
            if source.is_symlink() or not source.is_file() or source.stat().st_size>2*1024*1024:
                raise ValueError("unsafe crate notice path: "+section_title+"/"+filename)
            data=source.read_bytes()
            if len(data)==0:
                raise ValueError("empty crate license file: "+section_title+"/"+filename)
            try:
                original=data.decode("utf-8")
            except UnicodeDecodeError as exc:
                raise ValueError("non UTF-8 original crate license: "+section_title+"/"+filename) from exc
            sections.append("=== "+section_title+" / "+filename+" ===")
            sections.append("SPDX: "+str(rec["license_spdx"] or "UNKNOWN"))
            sections.append("SOURCE: https://crates.io/crates/"+rec["name"]+"/"+rec["version"])
            sections.append("ORIGINAL_FILE_SHA256: "+hashlib.sha256(data).hexdigest())
            sections.append(original)
            count+=1
        if not rec["notice_files"]:
            missing.append(section_title)
            sections.append("=== "+section_title+" / SOURCE LICENSE FILE MISSING ===")
            sections.append("SPDX: "+str(rec["license_spdx"] or "UNKNOWN"))
            sections.append("SOURCE: https://crates.io/crates/"+rec["name"]+"/"+rec["version"])
            sections.append("NOTICE: no standard LICENSE/NOTICE-like file in crate directory")
    sections.append("END_LICENSE_TEXTS")
    sections.append("MISSING_STANDARD_NOTICE_FILE_NAMES: "+", ".join(missing))
    raw=("\n".join(sections)+"\n").encode("utf-8")
    if len(raw)>16*1024*1024:
        raise ValueError("license aggregation over limit")
    output.parent.mkdir(parents=True,exist_ok=True)
    if output.exists():
        raise ValueError("refusing to overwrite existing third-party license bundle")
    output.write_bytes(raw)
    report["rust_source_license_bundle_sha256"]=hashlib.sha256(raw).hexdigest()
    report["rust_source_notice_file_count"]=count
    report["rust_source_package_notice_missing"]=missing
    print("G4_2_RUST_SOURCE_LICENSE_FILES=PASS COUNT="+str(count)+
          " MISSING_PACKAGES="+str(len(missing))+" BYTES="+str(len(raw))+
          " SHA256="+hashlib.sha256(raw).hexdigest())

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--upstream",required=True,type=Path)
    ap.add_argument("--output",required=True,type=Path)
    ap.add_argument("--inventory",required=True,type=Path)
    ap.add_argument("--license-bundle",required=True,type=Path)
    opts=ap.parse_args()
    root=opts.upstream.resolve()
    if command(["git","rev-parse","HEAD"],root).strip().lower()!=COMMIT:
        raise ValueError("unpinned upstream checkout")
    manifest=str(root/"llmfit-tui"/"Cargo.toml")
    tree=command(["cargo","tree","--locked","--target",TARGET,"--manifest-path",manifest,
                  "--prefix","none","--format","{p}","--edges","normal,build","--package","llmfit"],root)
    meta=json.loads(command(["cargo","metadata","--locked","--format-version","1",
                             "--filter-platform",TARGET,"--manifest-path",manifest],root))
    npm=json.loads((root/"llmfit-web"/"package-lock.json").read_text(encoding="utf-8"))
    workflow=(root/".github"/"workflows"/"release.yml").read_text(encoding="utf-8")
    builder=(root/"llmfit-tui"/"build.rs").read_text(encoding="utf-8")
    if not all(t in workflow for t in ("npm ci","npm run build","cargo build --release --target")):
        raise ValueError("upstream release build recipe changed")
    if not all(t in builder for t in ("llmfit-web","dist","include_bytes!")):
        raise ValueError("upstream embed recipe changed")
    report=collect(meta,tree,npm)
    static=opts.inventory.read_text(encoding="utf-8")
    if static.count("BEGIN_COMPONENTS\n") != 1 or static.count("\nEND_COMPONENTS") != 1:
        raise ValueError("static component inventory format invalid")
    actual=static.split("BEGIN_COMPONENTS\n",1)[1].split("\nEND_COMPONENTS",1)[0].splitlines()
    expected=[c["name"]+"@"+c["version"]+" | "+str(c["license_spdx"] or "UNKNOWN")+
              " | notices="+(",".join(c["notice_files"]) or "MISSING")
              for c in report["rust_packages"]]
    if actual!=expected:
        raise ValueError("static Windows CLI package inventory drifts from locked source")
    report["checked_component_inventory"]="packaging/LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt"
    generate_license_bundle(meta,report,opts.license_bundle)
    report["source_sha256"]={
      p:hashlib.sha256((root/p).read_bytes()).hexdigest()
      for p in ("Cargo.lock","llmfit-tui/Cargo.toml","llmfit-core/Cargo.toml",
                "llmfit-web/package-lock.json")}
    opts.output.parent.mkdir(parents=True,exist_ok=True)
    opts.output.write_text(json.dumps(report,ensure_ascii=False,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("G4_2_TARGET_CLI_AUDIT=PASS TARGET="+TARGET+
          " RUST_PACKAGES="+str(len(report["rust_packages"]))+" NPM_PRODUCTION=5")
    print("G4_2_RUST_LICENSE_SCOPE=BLOCKED FULL_TEXT_BUNDLED=false SIGNED_BINARY_GRAPH_ATTESTED=false")
    print("G4_2_TARGET_CLI_COMPONENTS_BEGIN")
    for c in report["rust_packages"]:
        print(c["name"]+"@"+c["version"]+" | "+str(c["license_spdx"] or "UNKNOWN")+
              " | notices="+(",".join(c["notice_files"]) or "MISSING"))
    print("G4_2_TARGET_CLI_COMPONENTS_END")

if __name__=="__main__":
    try:
        main()
    except (OSError,ValueError,subprocess.CalledProcessError,subprocess.TimeoutExpired) as exc:
        print("G4_2_TARGET_CLI_AUDIT=FAILED "+str(exc),file=sys.stderr)
        sys.exit(1)
