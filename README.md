## Geoserv

A server emulator for Endless Online developed in the Go programming language.

## Quick Start

   To set up and run geoserv for the first time, execute the following commands:
   

```bash
make build
./bin/geoserv
```

Pending database migrations are applied automatically on startup.

## Configuration

Configuration files for geoserv can be found in the 'config/' sub-directory of your current working directory.  

- server.yaml - Contains server configuration, database configuration, and account related items.
- gameplay.yaml - Game world configuration, combat configuration, npc's, guilds, etc.

You may create server.local.yaml or gameplay.local.yaml, which will allow you to override defaults without modifying any of the original files.

## Docker

```bash
make docker
docker compose up
```

## Requirements

- Go Version 1.25+
- EO Game data files located in 'data/' directory (maps, pub files)
