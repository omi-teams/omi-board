# v1 contract and generation

`openapi.yaml` is the source for Go server interfaces/models, web TypeScript
request/response types and the Python client. Edit the contract, then run
`make gen` (Go and npm must be installed; run `npm ci` in `web` first).
Go tools and npm packages are pinned in their respective manifests/lockfiles.
The web manifest overrides openapi-typescript's TypeScript 5 peer range to
use the existing TypeScript 6 compiler; generation and the web build verify it.
`make gen-check` regenerates and fails on changed or missing output.
`make test-gen` checks repeatability, deliberately changes a response description,
asserts drift is detected, restores the files, and runs the Python client tests.
CI also checks Git diff/status after generation to catch unexpected output files.

The Python generator supports OpenAPI 3.0.3, GET and POST paths under `/v1` without
parameters, named closed object schemas with string/integer/boolean fields or a
`$ref` to a plain model (one level deep), a required JSON `$ref` request body on POST,
cookie `apiKey` security schemes, and 200 JSON / 204 empty success responses with
401/403/503 Problem errors. Optional fields may be absent but cannot be null. Other
features fail generation, so extend the generator with tests before extending the
contract beyond this subset.
The Python client uses only the standard library at runtime; methods use the
contract's operation IDs (for example `Client(base_url).getV1Healthz()`). The client keeps
session cookies in `client.cookies`; set `client.headers['X-CSRF-Token']` to the
`csrf_token` from login before calling a write operation such as `postV1AuthLogout()`.
The web client uses relative URLs by default.

WebSocket envelope documentation and its JSON Schema live in `events/`.
