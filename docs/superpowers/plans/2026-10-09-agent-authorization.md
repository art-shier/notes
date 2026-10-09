# Notes Agent authorization implementation plan

Goal: an Agent installs the online shiji-notes skill, opens Notes for human approval, and retains a private local credential for later requests.

Architecture: the client generates the credential locally and registers its hash. A ten-minute device grant links a private polling code to a public confirmation code. Only an authenticated browser session with CSRF protection can approve the grant and create a scoped token. Public skill resources are embedded in the Go binary.

Constraints: preserve legacy tokens, schema revision and environment-based clients; no ConfigHub; default https://notes.shier.art; no approval on the user's behalf; no production account changes.

Review focus: repeated approval or cancellation, expiry and revoked tokens, unknown resource paths, credential file ownership and replacement, compatibility with local skill modifications.

- [x] Backend: regression first failed with 404; additive migration and all auth endpoints implemented. SQLite and real PostgreSQL validate CSRF, scope, repeat approval, expiration, cancellation and cross-account approval race.
- [x] Client: private profile and environment compatibility verified on Windows and Linux; login/whoami/logout implemented, save verified through real CLI/browser integration.
- [x] Distribution: deterministic public skill assets and same-origin checksum installer implemented; generated resources checked in CI.
- [x] UI: three-step journey, complete copyable instructions, online links, account/code/permissions/expiry approval page; mobile screenshots and login/hash transitions verified with real browser.
- [x] Docs: canonical skill, API/OpenAPI, README and technical plan updated. Go race/vet, Python installer/storage, frontend build/tests and browser integration pass locally.
- [ ] Delivery: push reviewable branch/PR and check CI. Production still requires installing the new Notes version; no production approval or database edits were performed.
