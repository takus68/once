# AGENTS.md

## Project Overview

Once is a CLI/TUI tool for installing and managing web applications from Docker
images. It's designed to make self-hosting as easy as possible.

Once uses a proxy server (github.com/basecamp/kamal-proxy) to route traffic to
the application containers, which allows it to provide zero-downtime restarts
and upgrades, automatic SSL, and multiple applications running on a single
server. A single instance of Once may deploy one proxy container, along with
multiple application containers. Host-based routing is used inside the proxy to
route traffic to the correct application container.

## Code Architecture

## Design and Concepts

### Container Naming

All containers are namespaced (default namespace: "once"):
- Proxy: `{namespace}-proxy`
- Apps: `{namespace}-app-{appName}-{shortID}`

This allows us to easily identify the app containers, but still allows us to
boot a second app container without naming collisions when we want to deploy a
new version without downtime.

### Data and State

Once deals primarily with two classes of state:

- Application data: this is stored in Docker volumes to provide persistent
storage between versions of the app container. Once provides one volume for
each app, which it will mount into two locations (`/storage` and
`/rails/storage`) to match typical app conventions. Once may provide backup and
restore features on the contents of those volumes. But it does not otherwise
touch the volume contents, or have any opinions about what should be in there
-- this data is entirely for each app's own use.

- Configuration: this keeps track of the applications and settings that have
been set up using Once itself. For example, the list of applications that are
deployed, the hostname and TLS settings for each, any custom port settings for
the proxy, and so on. Once stores this information as JSON strings in an `once`
label on the containers and volumes, so that everything is stored within the
Docker state. Application settings go with the app container; proxy settings go
with the proxy container; volume settings (like encryption keys) go with the
volume.

## Build Commands

```bash
make build       # Build binary to bin/ (CGO disabled)
make test        # Run unit tests (internal/ packages)
make integration # Run integration tests (requires Docker)
make lint        # Run golangci-lint
```

Run a single test:
```bash
go test -v -run TestName ./internal/...
```

## Coding Style

- Always follow idiomatic Go style
- Don't use excessive comments. Try to make the code speak for itself; only resort to adding comments where the meaning or intention may otherwise be unclear, or a subtle detail may be missed.
- Organize imports in sections: stdlib imports, 3rd-party imports, and project imports. Each section should be sorted alphabetically, and the sections should be separated from each other by 1 blank line.
- Where a struct type has both public and private methods, arrange the public methods first, then add a blank line, and then the private methods.

## Personal Notes

- I'm using this fork primarily to learn how kamal-proxy integration works.
- Useful reference: the proxy container lifecycle is managed in `internal/proxy`.
