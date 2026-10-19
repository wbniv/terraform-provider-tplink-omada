#!/usr/bin/env python3
"""Initialize and inspect the disposable local controller; never target a live URL."""

import argparse
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import ssl
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
RUNTIME = ROOT / ".controller-lab"
BASE = "https://127.0.0.1:18043"
VERSION = "6.1.0.19"


def private_json(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n")
    path.chmod(0o600)


class LabAPI:
    def __init__(self):
        # Bind all mutations to the compose-managed lab, not an environment URL.
        info = json.loads(subprocess.check_output([
            "docker", "inspect", "omada-provider-lab-controller-1"
        ]))[0]
        if info["Config"]["Labels"].get("omada.provider.lab") != "true":
            raise RuntimeError("Controller is not the managed lab")
        RUNTIME.mkdir(mode=0o700, exist_ok=True)
        self.token = ""
        self.controller_id = ""
        self.opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}),
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
            urllib.request.HTTPSHandler(context=ssl._create_unverified_context()),
        )

    def request(self, path, method="GET", data=None, *, anonymous=False, allow_error=False):
        route = path if path == "/api/info" else "/" + self.controller_id + path
        headers = {"Content-Type": "application/json"}
        if self.token and not anonymous:
            route += ("&" if "?" in route else "?") + urllib.parse.urlencode({"token": self.token})
            headers["Csrf-Token"] = self.token
        body = None if data is None else json.dumps(data).encode()
        req = urllib.request.Request(BASE + route, body, headers, method=method)
        try:
            response = self.opener.open(req, timeout=60)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            status = response.status
        try:
            payload = json.loads(raw)
        except ValueError:
            payload = {}
        private_json(RUNTIME / "last-response.json", payload)
        result = payload.get("result")
        event = {"method": method, "path": path.split("?")[0], "http_status": status,
                 "error_code": payload.get("errorCode"), "body_bytes": len(raw),
                 "result_type": type(result).__name__}
        if isinstance(result, dict):
            event["result_keys"] = sorted(result)
            event["pagination"] = {k: result[k] for k in ("totalRows", "currentPage", "currentSize") if k in result}
            if isinstance(result.get("data"), list):
                event["rows"] = len(result["data"])
        elif isinstance(result, list):
            event["rows"] = len(result)
        with (RUNTIME / "api-events.jsonl").open("a") as output:
            output.write(json.dumps(event) + "\n")
        if not allow_error and (status < 200 or status >= 300 or payload.get("errorCode") != 0):
            raise RuntimeError(f"{method} {path.split('?')[0]}: HTTP {status}, API {payload.get('errorCode')}")
        return payload

    def discover(self, wait=False):
        deadline = time.monotonic() + (300 if wait else 1)
        while True:
            try:
                info = self.request("/api/info", anonymous=True)["result"]
                break
            except (OSError, RuntimeError):
                if time.monotonic() >= deadline:
                    raise
                time.sleep(2)
        if info["controllerVer"] != VERSION:
            raise RuntimeError("Unexpected controller version")
        self.controller_id = info["omadacId"]
        return info

    def credentials(self):
        data = json.loads((RUNTIME / "credentials.json").read_text())
        if data["controller_id"] != self.controller_id:
            raise RuntimeError("Lab identity changed; remove old runtime credentials after stopping the old lab")
        return data

    def login(self):
        credentials = self.credentials()
        result = self.request("/api/v2/login", "POST", {
            "username": credentials["username"], "password": credentials["password"]
        }, anonymous=True)["result"]
        self.token = result["token"]


def initialize(api):
    info = api.discover(wait=True)
    path = RUNTIME / "credentials.json"
    if not path.exists():
        if info.get("registeredRoot") or info.get("configured"):
            raise RuntimeError("Refusing initialization of an existing account without lab credentials")
        private_json(path, {"controller_id": api.controller_id, "username": "lab-admin",
                            "password": "Lab-" + secrets.token_urlsafe(24),
                            "device_username": "lab-device", "device_password": "Device-A7-" + secrets.token_hex(8)})
    credentials = api.credentials()
    if not info.get("registeredRoot"):
        api.request("/api/v2/anon/register", "POST", {
            "username": credentials["username"], "password": credentials["password"],
            "email": "lab@example.invalid", "allowRemoteBinding": False, "language": 1,
        }, anonymous=True)
    if not info.get("configured"):
        defaults = api.request("/api/v2/anon/controllerInfo", anonymous=True)["result"]
        timezone = defaults.get("timeZone") or "UTC"
        region = defaults.get("region") or "United States"
        api.request("/api/v2/anon/cloud/status", "POST", {"enable": False}, anonymous=True)
        api.request("/api/v2/anon/cmd/wizardConfig", "POST", {
            "controllerName": "Terraform Provider Lab", "timeZone": timezone, "region": region,
            "controllerNotification": False, "cloudFirmwareDetection": False,
            "expImprove": False, "msp": False,
            "site": {"siteName": "Provider Lab", "scenario": "Office", "timeZone": timezone,
                     "region": region, "type": 0, "supportL2": True, "supportES": False},
            "deviceAccountSetting": {"username": credentials["device_username"], "password": credentials["device_password"]},
        }, anonymous=True)
    api.login()
    sites = api.request("/api/v2/sites?currentPage=1&currentPageSize=100")["result"]
    rows = sites["data"] if isinstance(sites, dict) else sites
    site = next(site for site in rows if site["name"] == "Provider Lab")
    credentials["site_id"] = site["id"]
    private_json(path, credentials)
    print(f"Initialized Omada {VERSION}: local test account, one site, no adopted devices.")


def probe(api):
    api.discover()
    api.login()
    site = api.credentials()["site_id"]
    paths = ["/api/v2/sites", "/api/v2/idps", "/api/v2/extendUserGroups"]
    paths += [f"/api/v2/sites/{site}" + suffix for suffix in (
        "/setting/lan/networks", "/setting/wlans", "/setting/lan/profiles",
        "/setting/firewall/acls?type=0", "/setting/firewall/ipGroups", "/setting/service/mdns", "/devices",
        # Diagnostic UI routes; these are not the provider's IP-group implementation.
        "/setting/profiles/groups", "/setting/profiles/groups/0",
    )]
    groups = api.request(f"/api/v2/sites/{site}/setting/wlans")["result"]["data"]
    if groups:
        paths.append(f"/api/v2/sites/{site}/setting/wlans/{groups[0]['id']}/ssids")
    for path in paths:
        separator = "&" if "?" in path else "?"
        previous = set()
        for page in (1, 2):
            payload = api.request(path + separator + f"currentPage={page}&currentPageSize=1", allow_error=True)
            result = payload.get("result")
            rows = result.get("data") if isinstance(result, dict) else result
            values = [row.get("id", row.get("idpId")) for row in rows] if isinstance(rows, list) else []
            identities = {value for value in values if isinstance(value, str) and value}
            summary = {"path": path.replace(site, "<site>"), "requested_page": page,
                       "code": payload.get("errorCode"), "shape": type(result).__name__,
                       "rows": len(rows) if isinstance(rows, list) else None,
                       "identified_rows": sum(isinstance(value, str) and bool(value) for value in values),
                       "repeated_ids": len(identities & previous)}
            if groups:
                summary["path"] = summary["path"].replace(groups[0]["id"], "<group>")
            if isinstance(result, dict):
                summary.update({k: result[k] for k in ("totalRows", "currentPage", "currentSize") if k in result})
            print(json.dumps(summary))
            previous = identities


def environment(api):
    api.discover()
    credentials = api.credentials()
    values = {
        "TF_ACC": "1", "TF_ACC_TERRAFORM_PATH": "/usr/local/bin/terraform",
        "OMADA_CONTROLLER_LAB": "1", "OMADA_URL": "https://controller:8043",
        "OMADA_USERNAME": credentials["username"], "OMADA_PASSWORD": credentials["password"],
        "OMADA_SKIP_TLS_VERIFY": "true", "OMADA_TEST_SITE_ID": credentials["site_id"],
        "OMADA_LAB_DEVICE_PASSWORD": credentials["device_password"],
    }
    path = RUNTIME / "runner.env"
    path.write_text("".join(f"{key}={value}\n" for key, value in values.items()))
    path.chmod(0o600)
    print("Private runner environment prepared.")


if __name__ == "__main__":
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["initialize", "probe", "environment"])
    args = parser.parse_args()
    api = LabAPI()
    {"initialize": initialize, "probe": probe, "environment": environment}[args.command](api)
