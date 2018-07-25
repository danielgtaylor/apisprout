<img src="https://user-images.githubusercontent.com/106826/43119494-78be9224-8ecb-11e8-9d1a-9fc6f3014b91.png" width="300" alt="API Sprout"/>

A simple, quick, cross-platform API mock server that returns examples specified in an API description document. Features include:

- OpenAPI 3.x support
- Load from a URL or local file
- Accept header content negotiation
- Prefer header to select response to test specific cases
- Server name validation (enabled with `--validate-server`)
- Request parameter & body validation (enabled with `--validate-request`)
- Configuration via files, environment, or commandline flags

Usage is simple:

```sh
# Load from a local file
apisprout my-api.yaml

# Load from a URL
apisprout https://raw.githubusercontent.com/OAI/OpenAPI-Specification/master/examples/v3.0/api-with-examples.yaml
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
