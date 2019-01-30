# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]
- Put new items here.

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
