"""A3.4 private Windows acceptance: real native diagnostic through localhost UI.
No raw hardware JSON, session tokens, hostnames, paths or screenshots are logged.
"""
import contextlib
import http.cookiejar
import importlib.util
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import uuid

ROOT=Path(__file__).resolve().parents[2]
REFERENCE=ROOT/"reference-web"/"contracts"/"tests"/"test_contracts.py"

def expect(condition,message):
    if not condition:raise AssertionError(message)

def launch(binary):
    proc=subprocess.Popen([str(binary),"-root",str(ROOT),"-no-browser"],
        stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True,bufsize=1)
    line=proc.stdout.readline().strip()
    if not line.startswith("NEUROMETR_LOCAL_UI=http://127.0.0.1:"):
        proc.terminate()
        raise AssertionError("loopback-only UI did not start")
    url=line.split("=",1)[1]
    expect("?" not in url and "#" not in url, "token or query in UI URL")
    return proc,url

def stop(proc):
    if proc.poll() is None:
        proc.terminate()
        try:proc.wait(timeout=5)
        except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=3)
    proc.stdout.close()

def browser(url):
    jar=http.cookiejar.CookieJar()
    opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    return opener,jar

def request(opener,url,method="GET",data=None,headers=None):
    req=urllib.request.Request(url,data=data,method=method,headers=headers or {})
    try:
        with opener.open(req,timeout=18) as res:
            return res.status,res.read(2*1024*1024+100),dict(res.headers)
    except urllib.error.HTTPError as err:
        return err.code,err.read(2048),dict(err.headers)

def session(opener,url):
    code,html,headers=request(opener,url)
    expect(code==200 and "Проверить компьютер".encode() in html,"Russian UI failed")
    expect(headers.get("Access-Control-Allow-Origin") is None,"CORS enabled")
    expect("frame-ancestors 'none'" in headers.get("Content-Security-Policy",""),"CSP invalid")
    code,raw,_=request(opener,url+"api/session")
    expect(code==200,"secure session unavailable")
    csrf=json.loads(raw)["csrf"]
    expect(len(csrf)==64,"CSRF not cryptographically random")
    return csrf

def local_external_dial_denied(url):
    # Only test addresses assigned to this same CI runner; no remote requests.
    port=int(url.split(":")[-1].rstrip("/"))
    try:
        addresses={info[4][0] for info in socket.getaddrinfo(socket.gethostname(),None,socket.AF_INET)}
    except socket.gaierror:
        addresses=set()
    addresses={a for a in addresses if a!="127.0.0.1" and not a.startswith("127.")}
    if not addresses:
        print("A3_4_EXTERNAL_INTERFACE=SKIP no nonloopback IPv4 on hosted runner")
        return
    for ip in addresses:
        conn=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
        conn.settimeout(0.4)
        try:
            expect(conn.connect_ex((ip,port))!=0,"server accessible via nonloopback adapter")
        finally:conn.close()
    print("A3_4_EXTERNAL_INTERFACE=DENIED")

def verify_acl(directory):
    # Independent Windows ACL check; not based on mode bits and does not
    # disclose the private path or trustee names in CI logs.
    script=(
      "$ErrorActionPreference='Stop';"
      "$p=Join-Path $env:LOCALAPPDATA 'Neurometr\\history';"
      "$acl=Get-Acl -LiteralPath $p;"
      "$rules=@($acl.Access | Where-Object {"
      " $_.AccessControlType -eq 'Allow' -and "
      " ($_.IdentityReference.Value -match '^(Everyone|BUILTIN\\\\Users|NT AUTHORITY\\\\Authenticated Users|BUILTIN\\\\Guests)$')"
      "});"
      "if($rules.Count -gt 0){exit 3};exit 0"
    )
    p=subprocess.run(["powershell.exe","-NoProfile","-NonInteractive","-Command",script],
        stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=15)
    expect(p.returncode==0,"Windows history DACL has broad allowed trustees or could not be audited")
    expect(directory.is_dir(),"history folder missing")
    print("A3_4_WINDOWS_ACL_AUDIT=PASS")

def python_oracle(raw):
    spec=importlib.util.spec_from_file_location("frozen_web_validator",REFERENCE)
    module=importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    data=module.decode_bounded(raw,module.LIMIT_DIAGNOSTIC)
    module.validate("diagnostic",data)
    return data

def go_validate(raw):
    p=subprocess.run(["go","run","./cmd/contractvalidate","--root",str(ROOT),"--kind","diagnostic"],
                     input=raw,capture_output=True,cwd=ROOT,timeout=35)
    expect(p.returncode==0,"Go validator process failed")
    expect(json.loads(p.stdout)["valid"] is True,"Go full validator rejected saved report")

def main():
    expect(sys.platform=="win32","Windows x64 only")
    expect(REFERENCE.is_file(),"approved Web Python oracle not checked out")
    expect(len(sys.argv)==2,"CI binary path missing")
    binary=Path(sys.argv[1])
    dir=Path(os.environ["LOCALAPPDATA"])/"Neurometr"/"history"
    proc=None
    try:
        proc,url=launch(binary)
        opener,jar=browser(url)
        csrf=session(opener,url)
        local_external_dial_denied(url)
        origin=url.rstrip("/")
        unauthorized,_,_=request(opener,url+"api/history",headers={"Origin":"http://attacker.invalid"})
        expect(unauthorized==403,"cross-site GET accepted")
        forged,_,_=request(opener,url+"api/history",headers={"Host":"evil.invalid"})
        expect(forged==403,"Host rebinding accepted")
        absent,_,_=request(opener,url+"api/run",method="POST",data=b"{}",
            headers={"Content-Type":"application/json","Origin":origin})
        expect(absent==403,"CSRF-less POST accepted")
        foreign,_,_=request(opener,url+"api/run",method="POST",data=b"{}",
            headers={"Content-Type":"application/json","Origin":"http://evil.invalid",
                     "X-Neurometr-CSRF":csrf})
        expect(foreign==403,"cross-origin POST accepted")
        options,_,hdr=request(opener,url+"api/run",method="OPTIONS")
        expect(options==405 and "Access-Control-Allow-Origin" not in hdr,"CORS preflight accepted")
        get,_,_=request(opener,url+"api/run")
        expect(get not in (200,201),"GET caused diagnostic run")
        bad,_,_=request(opener,url+"api/report/../../secret")
        expect(bad!=200,"traversal opened a path")
        print("A3_4_HOST_ORIGIN_CSRF_CORS_GET=DENIED")
        status,created,_=request(opener,url+"api/run",method="POST",data=b"{}",
            headers={"Content-Type":"application/json","Origin":origin,
                     "X-Neurometr-CSRF":csrf})
        expect(status==201,"native diagnostic did not complete")
        runid=json.loads(created)["id"]
        uuid.UUID(runid)
        status,raw,_=request(opener,url+"api/report/"+runid)
        expect(status==200 and len(raw)<2*1024*1024,"saved native diagnostic invalid or oversized")
        report=python_oracle(raw)
        go_validate(raw)
        expect(report["catalog_ref"]["origin"]=="bundled","bundled catalog origin missing")
        expect(report["catalog_ref"]["version"]=="2026.11.11.2","catalog version drift")
        expect(report["catalog_ref"]["sha256"]=="1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2","catalog digest drift")
        expect(report["hardware"]["ram"]["physical_total_bytes"] is not None,"native Windows RAM not collected")
        expect(report["hardware"]["cpu"]["logical_processors"] is not None,"native Windows CPU not collected")
        expect(report["model_assessments"]==[] and report["runtime_checks"]==[] and report["cloud_checks"]==[],"invented model/cloud tests")
        expect(report["engines"]==[],"mandatory llmfit unexpectedly started")
        expect(report["hardware"]["npus"]==[],"fabricated NPU")
        for marker in (b'GITHUB_TOKEN',b'USERPROFILE',b'"hostname"',b'"username"',b'"serial_number"',b'"mac_address"',b'"ip_address"',b'private_key'):
            expect(marker not in raw,"PII or secret marker in report")
        status,exported,hdr=request(opener,url+"api/export/"+runid)
        expect(status==200 and exported==raw and "attachment" in hdr.get("Content-Disposition",""),"manual export differs from stored file")
        print("A3_4_NATIVE_WINDOWS=PASS FULL_GO_VALIDATOR=PASS WEB_PYTHON=PASS BUNDLED_REF=PASS NO_LLMFIT=PASS PII=PASS")
        print("A3_4_JSON_BYTES="+str(len(raw))+" GPU_ENTRIES="+str(len(report["hardware"]["gpus"])))
        verify_acl(dir)
        # Restart loses cookie/session, but not history. A newly issued session
        # must permit reading the exact same previously validated report.
        stop(proc);proc=None
        proc,url=launch(binary)
        opener,_=browser(url);session(opener,url)
        status,listed,_=request(opener,url+"api/history")
        expect(status==200,"history inaccessible after restart")
        history=json.loads(listed)
        expect(any(x["id"]==runid for x in history["entries"]),"valid history lost after restart")
        status,reopened,_=request(opener,url+"api/report/"+runid)
        expect(status==200 and reopened==raw,"history reopened different bytes")
        # Corrupt a sibling without modifying the saved valid diagnostic.
        corrupt=dir/(str(uuid.uuid4())+".json")
        corrupt.write_bytes(b"{")
        unsupported=dir/(str(uuid.uuid4())+".json")
        unsupported.write_bytes(raw.replace(b'"schema_version":"0.1.0"',b'"schema_version":"99.0.0"',1))
        oversized=dir/(str(uuid.uuid4())+".json")
        oversized.write_bytes(b"x"*(2*1024*1024+1))
        status,listed,_=request(opener,url+"api/history")
        lst=json.loads(listed)
        expect(status==200 and lst["invalid_count"]>=3,"malformed historical report silently accepted")
        expect(any(x["id"]==runid for x in lst["entries"]),"corrupt entry blocked valid neighbor")
        print("A3_4_HISTORY_RESTART=PASS CORRUPT_ISOLATION=PASS UNSUPPORTED_VERSION=REJECTED OVERSIZE=REJECTED")
        # No runner diagnostics are attached to Actions; all raw payloads stay
        # in the per-user ephemeral Windows profile and memory only.
    finally:
        if proc is not None:stop(proc)

if __name__=="__main__":
    main()
