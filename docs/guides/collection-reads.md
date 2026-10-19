# Complete collection reads

The provider reads all pages for sites, LAN networks, WLAN groups, SSIDs, port profiles, ACL rules, IP groups, mDNS rules, SAML identity providers, and SAML roles. Raw SSID reads used to construct updates also read all pages and preserve returned attributes. Site resolution, resource lookups, and data sources inherit these complete reads. The devices endpoint retains its direct-array behavior.

Requests begin with page 1 and a page size of 100. The provider preserves ACL filters and authentication on subsequent pages. A smaller returned page does not establish completion: a controller may cap the requested size.

## Supported response shapes

- A direct array on the first request is treated as the complete collection.
- A page object must have an array `data`. When present, `totalRows` must be a nonnegative integer and remain stable across requests; collection completion requires that many unique objects. A supplied `currentPage` must match the request. `currentSize` is checked for a nonnegative integer but is not used to infer completion.
- Without `totalRows`, the provider continues until an empty page. An API that returns only a data wrapper must advance pages and eventually return an empty array; otherwise the read fails.
- Every object must have a unique, nonempty string identity (`id`, or `idpId` for SAML identity providers).

Invalid metadata, duplicate identities/fields, unexpected response-shape changes, early empty pages, and later-page HTTP/API failures fail the entire read. The provider returns no partial collection. Errors use fixed protocol descriptions or numeric HTTP/API codes, excluding raw response values.

A collection is limited to 10,000 pages and 64 MiB of cumulative JSON `result` data. The existing 16 MiB limit on each HTTP response also applies. Exceeding a limit reports an error. These bounds prevent an unending or oversized listing from being accepted as complete.

## Refresh and recovery

An object beyond the first page can now be imported or refreshed. A read failure keeps the existing managed identity and reports a diagnostic. Fix authorization, controller/proxy availability, or pagination consistency and retry Terraform. Concurrent inventory changes can cause a read to fail if totals or identities become inconsistent; retry once the controller settles. Pagination alone cannot establish a point-in-time snapshot without controller support.

Schemas and import IDs are unchanged. This patch does not remove missing resources from Terraform state: confirmed disappearance is a separate follow-up. A raw HTTP 404 remains an error, not confirmation of deletion.

Behavior is covered by synthetic client and Terraform tests. Real controller versions and zero-device behavior must be recorded separately when validated with the Omada Software Controller.
