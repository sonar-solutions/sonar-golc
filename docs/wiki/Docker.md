# Executable binaries

You can install from the stable release by clicking [here](https://github.com/sonar-solutions/sonar-golc/releases/tag/V1.0.9).

# With Docker

Use the published image **`timothe/sonar-golc`** from Docker Hub. Config is provided via a mounted directory; the web UI is served on port 8092. The DevOps platform to analyze is set with the **`GOLC_DEVOPS`** environment variable (e.g. `Github`, `Gitlab`, `BitBucket`, `File`).

**Run with the published image:**

```bash
mkdir -p config && cp config_sample.json config/config.json
# Edit config/config.json with your tokens and organization

docker run -p 8092:8092 \
  -v "$(pwd)/config:/config:ro" \
  -e GOLC_DEVOPS=Github \
  timothe/sonar-golc
```

To persist results on the host, add: `-v "$(pwd)/data:/data"`. After the analysis completes, open **http://localhost:8092** to view the results. View logs with `docker logs <container>`.

**Building the image** (optional, e.g. for local development): `docker build -t sonar-golc .` — then use `sonar-golc` instead of `timothe/sonar-golc` in the commands below.

**Docker Compose:** Put `config.json` in a `config/` directory. The Compose file uses the image `sonar-golc` (pull the published image and tag it, or use your built image):

```bash
docker pull timothe/sonar-golc && docker tag timothe/sonar-golc sonar-golc
mkdir -p config && cp config_sample.json config/config.json
# Edit config/config.json, then:
docker compose up
```

**Docker Compose variables:** Set these in a `.env` file in the project root or pass them when running (e.g. `GOLC_DEVOPS=Gitlab docker compose up`).

| Variable       | Default   | Description                                                                 |
|----------------|-----------|-----------------------------------------------------------------------------|
| `GOLC_DEVOPS`  | `Github`  | DevOps platform to analyze. Must match a key in your config (e.g. `Github`, `Gitlab`, `BitBucket`, `BitBucketSRV`, `Azure`, `File`). |

The Compose file mounts `./config` read-only; `/data` uses an anonymous volume so results persist across restarts.

---

## Full Docker and Compose guide

*(From `docs/docker.md` in the repository.)*

The project includes a multi-stage Dockerfile that produces a lean image (Alpine-based) for running GoLC. Config lives on a mounted volume; results and logs are written to a data volume; the web UI is served on port 8092. The DevOps platform to analyze is set via the **`GOLC_DEVOPS`** environment variable (e.g. `Github`, `Gitlab`, `BitBucket`, `File`). All application output goes to stdout/stderr so `docker logs` shows it.

### Build

```bash
docker build -t sonar-golc .
```

### Run

#### Required mounts

- **Config** — Mount a directory that contains `config.json` at `/config` (read-only). The container reads `/config/config.json` (override with `GOLC_CONFIG_FILE`).
- **Data** — Mount a writable directory or named volume at `/data`. GoLC writes `Results/` and `Logs/` there; the entrypoint copies static assets there for the web UI.

#### Example with bind mounts

```bash
# Create directories
mkdir -p ./config ./data
cp config_sample.json ./config/config.json
# Edit ./config/config.json with your tokens and organization

docker run -p 8092:8092 \
  -v "$(pwd)/config:/config:ro" \
  -v "$(pwd)/data:/data" \
  -e GOLC_DEVOPS=Github \
  sonar-golc
```

#### Example with named volume for data

```bash
docker volume create golc-data
docker run -p 8092:8092 \
  -v "$(pwd)/config:/config:ro" \
  -v golc-data:/data \
  -e GOLC_DEVOPS=Github \
  sonar-golc
```

#### Environment variables

| Variable           | Default               | Description                                                                 |
|--------------------|-----------------------|-----------------------------------------------------------------------------|
| `GOLC_CONFIG_FILE` | `/config/config.json` | Path to the config file inside the container.                              |
| `GOLC_DEVOPS`      | `Github`              | DevOps platform to analyze (e.g. `Github`, `Gitlab`, `BitBucket`, `File`). Must match a key in your config. |
| `PORT`             | `8092`                | Port the ResultsAll server listens on.                                    |

#### Accessing the UI and logs

- After the analysis finishes, the ResultsAll server starts. Open **http://localhost:8092** in a browser to view the report.
- View application logs: `docker logs <container_id_or_name>`

### Bind mount permissions

The container runs as a non-root user. If you use a bind mount for `/data` and get permission errors, ensure the host directory is writable by that user (e.g. `chmod 777 ./data` for testing, or match the container user UID).

### Port already in use

If port 8092 is in use, set `PORT` to another value and publish that port:

```bash
docker run -p 9092:9092 \
  -v "$(pwd)/config:/config:ro" \
  -v "$(pwd)/data:/data" \
  -e PORT=9092 \
  -e GOLC_DEVOPS=Github \
  sonar-golc
```

Then open http://localhost:9092.

### Docker Compose

A `docker-compose.yml` is provided so you can run GoLC with one command. It uses the pre-built image `sonar-golc` (build locally with `docker build -t sonar-golc .` or pull a published image and tag it).

#### Setup

1. Create a `config` directory and put your `config.json` in it (e.g. `cp config_sample.json config/config.json` then edit).
2. Optionally set the DevOps platform via a `.env` file or when running:

```bash
# .env (optional)
GOLC_DEVOPS=Github
```

Valid values match the keys in your config (e.g. `Github`, `Gitlab`, `BitBucket`, `BitBucketSRV`, `Azure`, `File`).

#### Run with Compose

```bash
# Ensure image exists: docker build -t sonar-golc .  (or pull and tag)
docker compose up
```

`GOLC_DEVOPS` defaults to `Github`; override with `.env` or: `GOLC_DEVOPS=Gitlab docker compose up`.

Results and logs are written to `/data` inside the container. The Dockerfile declares `VOLUME ["/data"]`, so Docker uses an anonymous volume for `/data` and results persist across container restarts. After the analysis completes, open **http://localhost:8092** to view the report.

#### Compose options

| Option                                  | Description |
|-----------------------------------------|-------------|
| `docker compose up`                     | Run using image `sonar-golc`. |
| `docker compose up --build`             | Build the image first (add `build: .` to the service if you want this). |
| `docker compose logs -f sonar-golc`     | Stream container logs. |

The Compose file mounts `./config` at `/config` (read-only). `/data` is not mounted in the file, so the Dockerfile's `VOLUME ["/data"]` applies and an anonymous volume is used. To use a named or bind mount for data (e.g. to access results on the host), add to the service: `volumes: - golc-data:/data` and define `volumes: golc-data:` at the bottom, or `- ./data:/data`.

### Image details

- **Build stage:** `golang:1.23-alpine` — builds both `golc` and `ResultsAll` with static linking.
- **Run stage:** `alpine:3.19` — minimal base with CA certificates (for HTTPS to GitHub/Bitbucket/GitLab/Azure) and a shell for the entrypoint.
- **Flow:** On start, the entrypoint copies static assets into `/data`, runs `golc -devops $GOLC_DEVOPS` (using config from `/config`), then starts the ResultsAll server. If config is invalid or golc fails, the container exits and the server does not start.
