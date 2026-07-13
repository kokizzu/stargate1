
# Stargate + Cassandra Example

Example showing how to use Stargate with Cassandra from Go. The full
integration test starts Cassandra and Stargate containers through
`ory/dockertest`, so the default test path stays local and does not start
Docker.

## Requirements

- Go 1.26 or newer
- Docker for the integration test

## Usage

Run local sanity tests:

```sh
make test
```

Run the container integration test:

```sh
RUN_STARGATE_INTEGRATION=1 make integration-test
```

Run a dependency vulnerability check:

```sh
make vulncheck
```

Run any one-off command through the Makefile:

```sh
make run CMD='go test ./...'
```

## Maintenance Checklist

- [x] Update the Go runtime directive to Go 1.26.
- [x] Keep the Cassandra/Stargate test opt-in with an integration build tag.
- [x] Add a local sanity test that does not start Docker.
- [x] Add Makefile targets for tests, integration tests, vulnerability checks, and arbitrary commands.
- [x] Run `make test`.
- [x] Run `make vulncheck`; no reachable vulnerabilities were found.
