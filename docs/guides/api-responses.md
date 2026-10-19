# API response and error handling

The provider accepts an API acknowledgment only when the HTTP status is 2xx and the response contains one JSON object with an explicit integer `errorCode` equal to zero. This applies to controller discovery, login, ordinary resource operations, and certificate uploads.

An HTTP error is never a successful operation, even when its JSON body looks like an Omada success response. Missing/null/noninteger codes, duplicate envelope fields, malformed/trailing JSON, and responses larger than 16 MiB are rejected. An explicit success envelope may omit `result` or contain a null/empty result for operations that do not require returned data. Entirely empty bodies, including HTTP 204, are rejected: no endpoint-specific empty-body exception has been established.

## Controller URLs and session expiry

Configure the final controller API URL directly. Redirects are rejected, including redirects within the same host, so the provider does not forward credentials or replay a mutation at another URL.

An explicit Omada session-expiry code (`-1`) in an HTTP 2xx envelope triggers one authentication refresh and one replay. HTTP 401, 403, and server errors do not trigger this replay, even if the body contains `-1`. If those statuses occur, inspect authentication, controller, and proxy configuration before retrying.

## Diagnostics and recovery

Errors include HTTP status or Omada code when available. Arbitrary server messages, response bodies, headers, URLs, and raw transport messages are excluded because they may contain credentials. Use the numeric code and controlled controller/proxy logs to investigate; automation must not depend on the old raw message text. Go callers can inspect `HTTPError`, `APIError`, `ProtocolError`, and `TransportError` with `errors.As`; transport causes remain available through `errors.Is`/`errors.As`.

A failed DELETE produces an error and keeps the resource in Terraform state. Correct the underlying failure and retry. HTTP 404 alone is not treated as proof of absence: a proxy or unsupported route may also return it. Complete pagination and resource-specific disappearance handling are separate work.

A rejected or malformed response does not establish whether a remote mutation happened. Verify the controller before retrying an ambiguous write. Resource-specific partial-write recovery remains relevant; for example, site settings retain the accepted new password when a valid PATCH acknowledgment is followed by a readback failure.

## Compatibility

The schema and import formats are unchanged. Configurations relying on redirected URLs must use the final URL. Unexpected empty responses and nonstandard envelopes now fail explicitly, and raw server message text is no longer part of diagnostics. Verify actual envelopes, expiry behavior, large responses, and proxy configuration on the target controller/firmware before deployment. Local mock tests do not establish firmware compatibility.
