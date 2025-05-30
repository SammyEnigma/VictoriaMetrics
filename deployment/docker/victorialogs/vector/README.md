# Docker compose Vector integration with VictoriaLogs

The folder contains examples of [Vector](https://vector.dev/docs/) integration with VictoriaLogs using protocols:

* [elasticsearch](./elasticsearch)
* [loki](./loki)
* [jsonline single node](./jsonline)
* [jsonline HA setup](./jsonline-ha)
* [datadog](./datadog)

## Quick start

To spin-up environment `cd` to any of listed above directories run the following command:
```sh
docker compose up -d 
```

To shut down the docker-compose environment run the following command:
```sh
docker compose down -v
```

The docker compose file contains the following components:

* vector - logs collection agent configured to collect and write data to `victorialogs`
* victorialogs - logs database, receives data from `vector` agent
* victoriametrics - metrics database, which collects metrics from `victorialogs` and `vector` for observability purposes

## Querying

* [vmui](https://docs.victoriametrics.com/victorialogs/querying/#vmui) - a web UI is accessible by `http://localhost:9428/select/vmui`
* for querying the data via command-line please check [vlogscli](https://docs.victoriametrics.com/victorialogs/querying/#command-line)

Vector configuration example can be found below:
* [elasticsearch](./elasticsearch/vector.yml)
* [loki](./loki/vector.yml)
* [jsonline single node](./jsonline/vector.yml)
* [jsonline HA setup](./jsonline-ha/vector.yml)
* [datadog](./datadog/vector.yml)

> Please, note that `_stream_fields` parameter must follow recommended [best practices](https://docs.victoriametrics.com/victorialogs/keyconcepts/#stream-fields) to achieve better performance.
