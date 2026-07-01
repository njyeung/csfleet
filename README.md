# CSFLEET

### Orchestration for Counter Strike 2 servers

Features:
- Lifecycle management for containerized servers, including auto-restart
- Declarative toml files for automatically loading plugins
- UDP proxy to docker network
- Clusters are a group of 0 or more servers that become load balance per UDP flow (client session) based on user defined policies (round robin, packing)
- Auto provisioning on orchestrator bootup so the game and plugins always stay updated
- Overlay fs with a base layer that includes the game and counter strike sharp.

System requirements:
- Linux (we use overlay fs and nftables)
- Docker (we use the cs2 dedicated server)
- 60GB of free storage (game only needs 60GB but plugins will need more)
- 2 CPUs
- 2 GiB RAM

## Build and deploy

The deployable runtime package contains only:

- `csfleet`
- `frontend/build`
- `.env.example`

Runtime state stays on the server next to the binary: `.env`, `base/`,
`instances/`, and `cache/` are not included in the package.

Build the package locally:

```sh
./scripts/package-release.sh
```

This writes `dist/csfleet-linux-amd64.tar.gz`.

GitHub Actions builds the same package on pushes to `main`. To publish a
downloadable release asset, push a version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

On the server, create `/opt/csfleet/.env` once, then update from the latest
release with:

```sh
sudo install -d -m 0755 /opt/csfleet
curl -L https://github.com/njyeung/csfleet/releases/latest/download/csfleet-linux-amd64.tar.gz -o /tmp/csfleet.tar.gz
sudo tar -xzf /tmp/csfleet.tar.gz -C /opt/csfleet --strip-components=1
sudo systemctl restart csfleet
```

Credit due to the following projects which CSFleet heavily relies on:

joedwards32 for providing the CS2 dedicated server image

CM2Walki for the SteamCMD image

Counter Strike Sharp
