![API Sprout](https://user-images.githubusercontent.com/106826/43119494-78be9224-8ecb-11e8-9d1a-9fc6f3014b91.png)

A simple, quick, cross-platform API mock server that returns examples specified in an OpenAPI 3.x document. Usage is simple:

```sh
apisprout my-api.yaml
```

## ToDo

[x] OpenAPI 3.x support
[x] Return defined examples
[ ] Validate request payload
[ ] Take `Accept` header into account to return the right media type
[ ] Generate fake data from schema if no example is available
[ ] Release binaries for Windows / Mac / Linux
[ ] Public Docker image

## License

Copyright &copy; 2018 Daniel G. Taylor

http://dgt.mit-license.org/
