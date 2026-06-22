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

Credit due to:

joedwards32 for providing the CS2 dedicated server image
CM2Walki for the SteamCMD image



