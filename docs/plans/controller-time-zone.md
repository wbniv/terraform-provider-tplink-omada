# Controller time zone management

## Goal

Make the time zone shown in Omada Controller **Global View → Settings → Controller Settings** manageable with Terraform. The current acceptance controller has one Omada site (`ES216G Acceptance`) and one adopted-device target (the ES216G switch).

## Current provider behavior

The provider already exposes a time zone on `omada_site.time_zone` and `omada_site_settings.timezone`. Those fields are documented as site settings. An Omada site is a logical grouping in a controller; it is not a per-switch setting. This test setup happens to have one site and one switch.

The controller-wide screen is separate in the UI. Do not add a second Terraform field or resource until the API behavior is verified: the existing site time zone may or may not write the same underlying setting shown on that screen.

## Observed state

On 2026-10-28, the controller-wide **Global View → Settings → Controller Settings** page showed a time-zone value of UTC. The user changed that setting manually in the UI. The final selected value and API readback have not been recorded, and the relationship to the provider's site-scoped fields remains unverified.

## Plan

### 1. Establish the setting's API identity

- [ ] Capture the controller's current global settings and the site's current settings through the API, with credentials and unrelated private values redacted.
- [ ] Change only the time zone in the controller UI and capture the resulting API requests and readback.
- [ ] Compare the before/after payloads with the existing `GetSite`, `GetSiteSettings`, `UpdateSite`, and site-settings API behavior.
- [ ] Determine whether `omada_site.time_zone` or `omada_site_settings.timezone` already controls the UI's controller-wide value, controls a distinct site value, or is unrelated.
- [ ] Record the endpoint, request shape, response shape, and controller-version evidence. Do not infer API semantics from matching field names.

### 2. Choose the Terraform ownership model

- [ ] If the existing site time zone field controls this UI setting, keep one source of truth and correct its docs/tests as needed. Do not add duplicate schema.
- [ ] If the controller-wide time zone is a distinct setting, add a controller-scoped singleton resource (proposed name: `omada_controller_settings`) with a `time_zone` attribute. Keep the existing site-scoped fields separate.
- [ ] If the controller API does not support reliable readback or safe partial updates, document the limitation and defer management rather than report false convergence.
- [ ] Define singleton import identity (for example, a fixed `controller` ID), import behavior, and delete semantics. Deleting the Terraform resource should relinquish state without resetting the controller's time zone.

### 3. Implement and document

- [ ] Add only the verified writable/readable time-zone field to the selected resource; avoid broad controller settings coverage in this change.
- [ ] Preserve settings outside the Terraform-managed field. If the API requires a whole-object write, use a verified read-modify-write contract and reject incomplete snapshots instead of filling unknown values with defaults.
- [ ] Add generated resource/data-source docs, examples, import instructions, and clarify the distinction between controller and site time zones.
- [ ] Ensure the resource can be used with the one-site, one-switch acceptance configuration without coupling controller-wide settings to switch lifecycle or site deletion.

### 4. Acceptance gates

- [ ] Client tests cover API payloads, decoding, omitted fields, malformed responses, and update failures.
- [ ] Terraform tests cover create/adopt, import, refresh, time-zone update, unchanged plan, out-of-band drift, and destroy-without-reset.
- [ ] Verify the existing `omada_site.time_zone` and `omada_site_settings.timezone` behavior remains stable and does not fight the new controller-scoped setting.
- [ ] Run a live check against the acceptance controller: import/read current value, apply one time-zone change, confirm UI and API readback, require a stable subsequent plan, then restore the intended value through Terraform.
- [ ] Record the tested Omada Controller version and the exact API behavior; do not generalize to other versions without evidence.

## Boundaries

- The initial scope is the time-zone field visible in the controller-wide settings screen, not all controller preferences.
- Do not change switch port configuration or adoption state as part of this work.
- Do not claim that a site time zone and controller time zone are the same until the API/UI comparison in step 1 proves it.
