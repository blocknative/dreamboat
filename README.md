

# 🛳️ Dreamboat

[![GoDoc](https://godoc.org/github.com/blocknative/dreamboat?status.svg)](https://godoc.org/github.com/blocknative/dreamboat)
[![Go Report Card](https://goreportcard.com/badge/github.com/blocknative/dreamboat?style=flat-square)](https://goreportcard.com/report/github.com/blocknative/dreamboat)
[![Discord](https://img.shields.io/discord/542403978693050389?color=%237289da&label=discord&style=flat-square)](https://img.shields.io/discord/542403978693050389?color=%237289da&label=discord&style=flat-square)

An Ethereum 2.0 Relay for proposer-builder separation (PBS) with MEV-boost.  Commissioned, built, and put to sea by Blocknative.

[![blocknative-dreamboat](https://user-images.githubusercontent.com/9452561/189974883-6910dd5c-0f55-4aa6-87be-515c5930362e.png)](https://blocknative.com)

## Quickstart

#### Installation

Install with `go get`, or download the latest release [here](https://github.com/blocknative/dreamboat/releases/latest).

```bash
$ go install github.com/blocknative/dreamboat
```

#### Usage

```bash
$ dreamboat --help
```

Documentation can be found [here](https://docs.blocknative.com/mev-relay-instructions-for-ethereum-validators).

## What is Dreamboat?

Dreamboat is a relay for Ethereum 2.0, it ships blocks from builders to validators.  Dreamboat is for anyone who wants to strengthen the Eth network by running their own relay.  Dreamboat's all-in-one design, modular code base and stunning performance will dazzle beginners and seasoned hackers alike.  All aboard!

More precisely, Dreamboat is a spec-compliant [builder](https://github.com/ethereum/builder-specs) and [relay](https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5) that allows block-builders and validators to interface with each other through [MEV-boost](https://github.com/flashbots/mev-boost).

Eventually Dreamboat will vanish, as if a dream, once in-protocol PBS is live.  Until then, it sails the ephemeral seas of block space, delivering its trusted cargo of blocks to validators, guided simply by a lighthouse.

## Design Goals
 
This is a from-scratch implementation of a PBS relay–aimed at strengthening #RelayDiversity–with the following design goals:

1. **Extensibility.**  Dreamboat provides well-factored interfaces that make it easy to change its behavior.  We use [Go Datastore](https://pkg.go.dev/github.com/ipfs/go-datastore) for its persistence layer, so you can swap out the default [BadgerDB](https://github.com/dgraph-io/badger) store with your favorite database, and provide a clean separation between core relay logic, the beacon-chain client, and the HTTP server.
2. **Reliability.** Work is ongoing implement a [supervision-tree architecture](https://ferd.ca/the-zen-of-erlang.html) in Dreamboat, so that you never miss a block.
3. **Performance.** We have put significant engineering effort into tuning Dreamboat's performance, and have much, much more in store.  The HTTP server's hot paths are 100% lock-free, concurrency is carefully tuned, and there are ongoing efforts to optimize the garbage collector and speed up signature validation.  The result is a ship that doesn't just sail... it *glides*.
4. **Transparency.**  Dreamboat implements the [Relay Data API](https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#38a21c8a40e64970904500eb7b373ea5), so you can audit past proposals.  But we take a much broader view of transparency, which includes **code transparency**.  A major cleanup effort is underway to make–and *keep*–Dreamboat's code *legible*.  We believe this is an essential part of ensuring transparency and reliability.

We are continuously improving Dreamoat's runtime performance, standards compliance, reliability and transparency. We would also like to thank the Flashbots team for their open-source tooling, which helped us get Dreamboat up and running in short order, and for their thoughtful comments on implementation.

## Dreamboat VS Others

Why should you opt for Dreamboat instead of other relays?

### Decentralized Bid Delivery
Unlike centralized relays that heavily rely on a single centralized source of truth, such as Redis, for bid propagation, the Relay project takes a distributed approach. We deliver bids to distributed instances, allowing for improved scalability, fault tolerance, and reduced reliance on a single point of failure. This decentralized bid delivery mechanism ensures higher availability and robustness of the relay network.

### Memory-Based Operations
One of the key differentiators of the Relay project is its heavy utilization of memory for serving various operations. Instead of relying on frequent database access, we leverage in-memory storage to serve most of the required data. This approach results in significantly faster response times and reduced latency for bid retrieval, payload decoding, and other essential operations.

### Efficient Data Storage
In contrast to relying solely on a costly PostgreSQL database for storing large volumes of data, the Relay project adopts a more cost-effective approach. We store increasing amounts of data in files, utilizing file-based storage systems. This strategy not only reduces infrastructure costs but also enhances performance and scalability, as file operations can be optimized and scaled more efficiently.

### Relay Diversity
Relay diversity plays a crucial role in ensuring the resilience, security, and efficiency of the overall network. By running relays with different configurations and settings, we can achieve a more diverse and robust network architecture.


## Command-line Flags

In addition to configuring the relay using the `config.ini` file (explained below), you can also use command-line flags to customize its behavior. The following flags are available:

- **`-loglvl`**: Sets the logging level for the relay. The default value is `info`. Possible options include `trace`, `debug`, `info`, `warn`, `error`, or `fatal`.
- **`-logfmt`**: Specifies the log format for the relay. The default value is `text`. Possible options include `text`, `json`, or `none`.
- **`-config`**: Specifies the path to the configuration file needed for the relay to run.
- **`-datadir`**: Sets the data directory where blocks and validators are stored in the default datastore implementation. The default value is `/tmp/relay`.

The following flags control specific relay functionalities:

- **`-relay-distribution`**: Enables running the relay as a distributed system with multiple replicas. By default, this flag is set to `false`.
- **`-warehouse`**: Enables warehouse storage of data. By default, this flag is set to `true`.


## Configuration Options

The relay system provides various configuration options that can be customized through the `config.ini` file, specified trough the flag `-config`. Each section and its respective items are explained below, along with their default values:

- **`[external_http]`**:
  - `address` (default: "0.0.0.0:18550"): The IP address and port on which the relay serves external connections.
  - `read_timeout` (default: 5s): The maximum duration for reading the entire request, including the body.
  - `idle_timeout` (default: 5s): The maximum amount of time to wait for the next request when keep-alives are enabled.
  - `write_timeout` (default: 5s): The maximum duration before timing out writes of the response.

- **`[internal_http]`**:
  - `address` (default: "0.0.0.0:19550"): The IP address and port on which the internal metrics profiling and management server operates.
  - `read_timeout` (default: 5s): The maximum duration for reading the entire request, including the body.
  - `idle_timeout` (default: 5s): The maximum amount of time to wait for the next request when keep-alives are enabled.
  - `write_timeout` (default: 5s): The maximum duration before timing out writes of the response.

- **`[api]`**:
  - `allowed_builders` (default: []): A comma-separated list of allowed builder public keys.
  - `submission_limit_rate` (default: 2): The rate limit for submission requests (per second).
  - `submission_limit_burst` (default: 2): The burst value for the submission request rate limit.
  - `limitter_cache_size` (default: 1000): The size of the rate limiter cache entries.
  - `data_limit` (default: 450): The limit of data (in bytes) returned in one response.
  - `errors_on_disable` (default: false): A flag indicating whether to return errors when the endpoint is disabled.

- **`[relay]`**:
  - `network` (default: "mainnet"): The name of the network in which the relay operates.
  - `secret_key` (default: ""): The secret key used to sign messages.
  - `publish_block` (default: true): A flag indicating whether to publish payloads to beacon nodes after delivery.
  - `get_payload_response_delay` (default: 800ms): The delay for responding to the GetPayload request.
  - `get_payload_request_time_limit` (default: 4s): The deadline for calling GetPayload.
  - `allowed_builders` (default: []): A comma-separated list of allowed builder public keys.

- **`[beacon]`**:
  - `addresses` (default: []): A comma-separated list of URLs to beacon endpoints.
  - `publish_addresses` (default: []): A comma-separated list of URLs to beacon endpoints for publishing.
  - `payload_attributes_subscription` (default: true): A flag indicating whether payload attributes should be enabled.
  - `event_timeout` (default: 16s): The timeout for beacon events.
  - `event_restart` (default: 5): The number of times to restart beacon events.
  - `query_timeout` (default: 20s): The timeout for beacon queries.

- **`[verify]`**:
  - `workers` (default: 2000): The number of workers running verify in parallel.
  - `queue_size` (default: 100000): The size of the verify queue.

- **`[validators]`**:
  - `db.url` (default: ""): The database connection query string.
  - `db.max_open_conns` (default: 10): The maximum number of open connections to the database.
  - `db.max_idle_conns` (default: 10): The maximum number of idle connections in the connection pool.
  - `db.conn_max_idle_time` (default: 15s): The maximum amount of time a connection can remain idle.
  - `badger.ttl` (default: 24h): The time-to-live duration for BadgerDB.
  - `queue_size` (default: 100000): The size of the response queue.
  - `store_workers` (default: 400): The number of workers storing validators in parallel.
  - `registrations_cache_size` (default: 600000): The size of the registrations cache.
  - `registrations_cache_ttl` (default: 1h): The time-to-live duration for the registrations read cache.
  - `registrations_write_cache_ttl` (default: 12h): The time-to-live duration for the registrations write cache.

Please note that these values can be modified in the `config.ini` file according to your specific requirements.

## Planned Features & Enhancements

The following features and enhancements are in-progress.

- [x] Support for multiple storage backends (Redis, Postgres, BadgerDB, Filesystem, etc.)
- [x] Runtime profiling & tracing endpoint
- [x] Parallel validator-signature verification
- [x] Contention-free execution along hot paths
- [x] Support for multiple beacon clients
- [ ] Support for multiple external block builders

## Extend the blockchain networks
Do you need to make your relay work with a different blockchain that is not:
- Mainnet
- Kiln
- Ropsten
- Sepolia
- Goerli

Then you need to extend the relay by creating a json file called `networks.json`, and put it inside `datadir` (`datadir` is a parameter you set when running the relay).

Inside the network.json file, you need to specify the network name and three other information:
- GenesisForkVersion
- GenesisValidatorsRoot
- BellatrixForkVersion

Here is an example of how you should format the networks:

```
{
    "blocknative":{
        "GenesisForkVersion": "",
        "GenesisValidatorsRoot":"",
        "BellatrixForkVersion": ""
    },
    "myNetwork":{
        "GenesisForkVersion": "",
        "GenesisValidatorsRoot":"",
        "BellatrixForkVersion": ""
    },
}
```

## Support

Stuck?  Don't despair!  Drop by our [discord](https://discord.com/invite/KZaBVME) to ask for help, or just to say hello. We're friendly! 👋


## Developing

### Building Locally

```
$ go run cmd/dreamboat
```


### Architecture

Proposer Builder Separation is a solution to shield validators from the centralizing force the black hole of MEV has released. The next chapter in this saga is to protect builders from too extreme of centralizing forces.

<img width="874" alt="PBS Context Diagram" src="https://user-images.githubusercontent.com/22778355/188292682-421dee2e-b11c-46ca-af08-7f6c0e0d665e.png">

## API Compatibility

Dreamboat follows semantic versioning.  Changes to the REST API are governed by the Flashbots specifications.

The `main` branch is **NOT** guaranteed to be stable.  Use the latest git tag in production!
