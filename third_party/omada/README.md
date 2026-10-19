# Vendored Omada Software Controller

The original TP-Link **6.1.0.19 Linux x64 distribution** is stored here through **Git LFS**, at Will's request. The archive is byte-for-byte unchanged. `manifest.json` records the official download URL, SHA-256, version, and the lab's runtime image. TP-Link's application and bundled dependencies retain their original notices and terms; this vendoring does not assign them the provider's license.

After cloning, install Git LFS, run `git lfs pull`, then verify:

```sh
python3 third_party/omada/verify.py
```

Expected SHA-256: `885eb2aeaed16b300e618297af2132d4e015c7f9bc5edce2ba81b8cdb181e126`.

The lab uses a digest-pinned community image for Java/MongoDB/service packaging. `dev/controller-lab/run.sh up` checks the vendored archive before starting; it then compares **all 533 application JARs** in the running container with this archive. A mismatch fails initialization. This verifies the controller application, not every file in the runtime image.

The LFS archive removes reliance on TP-Link retaining that download URL. Runtime Docker images remain separate, pinned dependencies. For an offline machine, cache/export the three images from `dev/controller-lab/compose.yaml` and `run.sh` with `docker image save`, transfer them, and use `docker image load`. The Terraform 1.15.8 binary must also be available. Go modules are separately vendored in the root `vendor/` directory, and lab compilation/test execution runs without an external network.

To update Omada deliberately: obtain the official archive, inspect the matching runtime package, update `manifest.json` and the Compose image together, verify every application JAR, and rerun the real-controller suite. Record failures and version-specific limitations before making a compatibility claim. Do not replace the existing archive silently or repoint its versioned filename.
