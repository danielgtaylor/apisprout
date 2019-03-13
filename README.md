<img src="https://user-images.githubusercontent.com/106826/43119494-78be9224-8ecb-11e8-9d1a-9fc6f3014b91.png" width="300" alt="API Sprout"/>

A simple, quick, cross-platform API mock server that returns examples specified in an API description document. Features include:

- OpenAPI 3.x support
  - Uses operation `examples` or generates examples from `schema`
- Load from a URL or local file (auto reload with `--watch`)
- CORS headers enabled by default
- Accept header content negotiation
  - Example: `Accept: application/*`
- Prefer header to select response to test specific cases
  - Example: `Prefer: status=409`
- Server validation (enabled with `--validate-server`)
  - Validates scheme, hostname/port, and base path
  - Supports `localhost` out of the box
- Request parameter & body validation (enabled with `--validate-request`)
- Configuration via:
  - Files (`/etc/apisprout/config.json|yaml`)
  - Environment (prefixed with `SPROUT_`, e.g. `SPROUT_VALIDATE_SERVER`)
  - Commandline flags

Usage is simple:

```sh
# Load from a local file
apisprout my-api.yaml

# Load from a URL
apisprout https://raw.githubusercontent.com/OAI/OpenAPI-Specification/master/examples/v3.0/api-with-examples.yaml
```

## Docker Image

A hosted [API Sprout Docker image](https://hub.docker.com/r/danielgtaylor/apisprout/) is provided that makes it easy to deploy mock servers or run locally. For example:

```sh
docker pull danielgtaylor/apisprout
docker run -p 8000:8000 danielgtaylor/apisprout http://example.com/my-api.yaml
```

Configuration can be passed via environment variables, e.g. setting `SPROUT_VALIDATE_REQUEST=1`, or by passing commandline flags. It is also possible to use a local API description file via [Docker Volumes](https://docs.docker.com/storage/volumes/):

```
# Remember to put the full path to local archive YAML in -v
docker run -p 8000:8000 -v $FULLPATH/localfile.yaml:/api.yaml danielgtaylor/apisprout /api.yaml
```

## Installation

Download the appropriate binary from the [releases](https://github.com/danielgtaylor/apisprout/releases) page.

Alternatively, you can use `go get`:

```sh
go get github.com/danielgtaylor/apisprout
```

## Contributing

Contributions are very welcome. Please open a tracking issue or pull request and we can work to get things merged in.

## Release Process

The following describes the steps to make a new release of API Sprout.

1. Merge open PRs you want to release.
1. Select a new semver version number (major/minor/patch depending on changes).
1. Update `CHANGELOG.md` to describe changes.
1. Create a commit for the release.
1. Tag the commit with `git tag -a -m 'Tagging x.y.z release' vx.y.z`.
1. Build release binaries with `./release.sh`.
1. Push the commit and tags.
1. Upload the release binaries.

## License

Copyright &copy; 2018 Daniel G. Taylor

http://dgt.mit-license.org/
