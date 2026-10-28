#!/usr/bin/env python3
"""Initialize and inspect only the labeled ES216G physical-test controller."""

import csv
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import ssl
import string
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
RUNTIME = ROOT / ".es216g-lab"
CONTAINER = "omada-es216g-controller"
VERSION = "6.1.0.19"
CREDENTIALS = RUNTIME / "credentials.json"
ENVIRONMENT = RUNTIME / "provider.env"
CSV_CREDENTIALS = RUNTIME / "controller-credential.csv"


def base_url():
    host = os.environ.get("ES216G_CONTROLLER_IP")
    if not host:
        raise RuntimeError("ES216G_CONTROLLER_IP must be set by run.sh")
    return f"https://{host}:8043"


def private_json(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n")
    path.chmod(0o600)


def device_password():
    value = "A7!z"
    alphabet = string.ascii_letters + string.digits + "-_"
    for _ in range(24):
        value += secrets.choice(alphabet.replace(value[-1], ""))
    return value


class Controller:
    def __init__(self):
        RUNTIME.mkdir(mode=0o700, exist_ok=True)
        RUNTIME.chmod(0o700)
        try:
            info = json.loads(subprocess.check_output(["docker", "inspect", CONTAINER]))[0]
        except (OSError, subprocess.CalledProcessError) as error:
            raise RuntimeError("The ES216G controller container is not running") from error
        if info["Config"]["Labels"].get("omada.provider.es216g") != "true":
            raise RuntimeError("Refusing to use an unowned controller container")
        self.opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}),
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
            urllib.request.HTTPSHandler(context=ssl._create_unverified_context()),
        )
        self.controller_id = ""
        self.token = ""

    def request(self, path, method="GET", data=None, *, anonymous=False):
        route = path if path == "/api/info" else "/" + self.controller_id + path
        headers = {"Content-Type": "application/json"}
        if self.token and not anonymous:
            route += ("&" if "?" in route else "?") + urllib.parse.urlencode({"token": self.token})
            headers["Csrf-Token"] = self.token
        body = None if data is None else json.dumps(data).encode()
        request = urllib.request.Request(base_url() + route, body, headers, method=method)
        try:
            response = self.opener.open(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status = response.status
            raw = response.read()
        try:
            payload = json.loads(raw)
        except ValueError as error:
            raise RuntimeError(f"Controller returned non-JSON from {path}") from error
        if status < 200 or status >= 300 or payload.get("errorCode") != 0:
            raise RuntimeError(f"Controller request failed: {method} {path} (HTTP {status}, API {payload.get('errorCode')})")
        return payload

    def discover(self, wait=False):
        deadline = time.monotonic() + (300 if wait else 1)
        while True:
            try:
                info = self.request("/api/info", anonymous=True)["result"]
                break
            except (OSError, RuntimeError, KeyError):
                if time.monotonic() >= deadline:
                    raise
                time.sleep(2)
        if info.get("controllerVer") != VERSION:
            raise RuntimeError(f"Expected Omada {VERSION}, found {info.get('controllerVer')}")
        self.controller_id = info["omadacId"]
        return info

    def credentials(self):
        data = json.loads(CREDENTIALS.read_text())
        if data["controller_id"] != self.controller_id:
            raise RuntimeError("Controller identity changed; refusing stale credentials")
        return data

    def login(self):
        credentials = self.credentials()
        result = self.request("/api/v2/login", "POST", {
            "username": credentials["username"],
            "password": credentials["password"],
        }, anonymous=True)["result"]
        self.token = result["token"]


def initialize(controller):
    info = controller.discover(wait=True)
    if not CREDENTIALS.exists():
        if info.get("registeredRoot") or info.get("configured"):
            raise RuntimeError("Controller is already initialized but has no lab-owned credentials; refusing to adopt it")
        private_json(CREDENTIALS, {
            "controller_id": controller.controller_id,
            "username": "es216g-admin",
            "password": "ES216G-" + secrets.token_urlsafe(24),
            "device_username": "es216g-device",
            "device_password": device_password(),
        })
    credentials = controller.credentials()
    if not info.get("registeredRoot"):
        controller.request("/api/v2/anon/register", "POST", {
            "username": credentials["username"],
            "password": credentials["password"],
            "email": "es216g-test@example.invalid",
            "allowRemoteBinding": False,
            "language": 1,
        }, anonymous=True)
    if not info.get("configured"):
        defaults = controller.request("/api/v2/anon/controllerInfo", anonymous=True)["result"]
        timezone = defaults.get("timeZone") or "Asia/Bangkok"
        region = defaults.get("region") or "Thailand"
        controller.request("/api/v2/anon/cloud/status", "POST", {"enable": False}, anonymous=True)
        controller.request("/api/v2/anon/cmd/wizardConfig", "POST", {
            "controllerName": "ES216G Terraform Acceptance",
            "timeZone": timezone,
            "region": region,
            "controllerNotification": False,
            "cloudFirmwareDetection": False,
            "expImprove": False,
            "msp": False,
            "site": {
                "siteName": "ES216G Acceptance",
                "scenario": "Office",
                "timeZone": timezone,
                "region": region,
                "type": 0,
                "supportL2": True,
                "supportES": True,
            },
            "deviceAccountSetting": {
                "username": credentials["device_username"],
                "password": credentials["device_password"],
            },
        }, anonymous=True)
    controller.login()
    sites = controller.request("/api/v2/sites?currentPage=1&currentPageSize=100")["result"]
    rows = sites["data"] if isinstance(sites, dict) else sites
    site = next((row for row in rows if row.get("name") == "ES216G Acceptance"), None)
    if site is None:
        raise RuntimeError("ES216G Acceptance site was not created")
    credentials["site_id"] = site["id"]
    private_json(CREDENTIALS, credentials)
    print(f"Initialized Omada {VERSION}; local site ready; cloud binding disabled.")
    print(f"Private credentials: {CREDENTIALS} (0600)")


def status(controller):
    info = controller.discover(wait=True)
    configured = bool(info.get("configured"))
    print(f"Omada {info.get('controllerVer')} at {base_url()}; configured={configured}")
    if CREDENTIALS.exists():
        controller.login()
        print(f"Lab site ID: {controller.credentials().get('site_id', 'not recorded')}")


def devices(controller):
    controller.discover(wait=True)
    controller.login()
    site = controller.credentials().get("site_id")
    result = controller.request(f"/api/v2/sites/{site}/devices")["result"]
    rows = result.get("data", []) if isinstance(result, dict) else result
    if not rows:
        print("No adopted devices in the ES216G Acceptance site.")
        return
    for row in rows:
        print(" ".join(str(row.get(key, "")) for key in ("mac", "name", "model", "firmwareVersion", "status")))


def write_environment(controller):
    controller.discover(wait=True)
    credentials = controller.credentials()
    lines = {
        "OMADA_URL": base_url(),
        "OMADA_USERNAME": credentials["username"],
        "OMADA_PASSWORD": credentials["password"],
        "OMADA_SKIP_TLS_VERIFY": "true",
        "OMADA_ES216G_SITE_ID": credentials["site_id"],
        "OMADA_ES216G_DEVICE_USERNAME": credentials["device_username"],
        "OMADA_ES216G_DEVICE_PASSWORD": credentials["device_password"],
    }
    ENVIRONMENT.write_text("".join(f"{key}={value}\n" for key, value in lines.items()))
    ENVIRONMENT.chmod(0o600)


def write_credentials_csv(controller):
    controller.discover(wait=True)
    credentials = controller.credentials()
    with CSV_CREDENTIALS.open("w", newline="") as output:
        writer = csv.writer(output)
        writer.writerow(["name", "url", "username", "password", "note"])
        writer.writerow([
            "Omada ES216G Acceptance",
            base_url(),
            credentials["username"],
            credentials["password"],
            "Local hardware acceptance controller",
        ])
    CSV_CREDENTIALS.chmod(0o600)


def main():
    if len(sys.argv) != 2 or sys.argv[1] not in {"initialize", "status", "devices", "environment", "credentials-csv"}:
        raise SystemExit("Usage: controller.py {initialize|status|devices|environment|credentials-csv}")
    controller = Controller()
    command = sys.argv[1]
    if command == "initialize":
        initialize(controller)
    elif command == "status":
        status(controller)
    elif command == "devices":
        devices(controller)
    elif command == "credentials-csv":
        write_credentials_csv(controller)
        print(f"Controller credential CSV written to {CSV_CREDENTIALS} (0600). Import it into Chrome, then shred the CSV.")
    else:
        write_environment(controller)
        print(f"Private provider environment written to {ENVIRONMENT} (0600).")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"es216g-lab: {error}", file=sys.stderr)
        raise SystemExit(1)
