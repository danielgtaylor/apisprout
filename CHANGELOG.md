# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]
- Return header examples when possible.
- Update dependency versions.

## [1.3.0] - 2019-03-18
- Add `--add-server` to add a custom server when using `--validate-server`.
  This allows quickly adding a custom domain or base path that will properly
  validate.
- Add `--header` (short `-H`) option to specify a custom header when fetching
  the API document. This allows you to pass custom auth info.
- Add `readOnly` and `writeOnly` support to the example generator.
- Revamped support for `--validate-server` (short `-s`)
  - Requires the use of server base path(s) on the client.
  - Localhost is now always allowed on all known base paths.
  - Support for proxy headers (e.g. `X-Forwarded-Host`).
- Better support for resolving relative path references.
- Be more resilient to parser panics when using `--watch`
- Update Docker build to use Go 1.12 and Go modules.
- Enhance example-from-schema generation code. Support enums, string formats,
  array and object examples, min/max and min items.

## [1.2.0] - 2019-02-27
- Add support for reloading OpenAPI URLs via `/__reload` on the server.
- Support external references in OpenAPI loader.
- Update dependencies, simplify file loading.
- Support jsonapi.org content type (`application/vnd.api+json`).
- Switch from `dep` to Go modules.

## [1.1.1] - 2019-01-30
- Fix `OPTIONS` request to also include CORS headers.

## [1.1.0] - 2019-01-29
- Added the `--watch` (short `-w`) parameter to reload whenever the input file
  changes. This currently only works when using files on disk.
- Update Docker build to use Go 1.11.
- Generate examples from schema when no example is available.
- Fix path parameter validation.
- Add CORS headers. Disable with `--disable-cors`.
- Documentation updates.

## [1.0.1] - 2018-10-03
- Dependency updates, fixes string format validation bug.

## [1.0.0] - 2018-07-24
- Initial release.
