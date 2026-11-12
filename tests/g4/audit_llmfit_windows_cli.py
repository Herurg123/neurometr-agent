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

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--upstream",required=True,type=Path)
    ap.add_argument("--output",required=True,type=Path)
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
