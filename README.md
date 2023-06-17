

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

## Key Differences and Benefits with Others

### Decentralized Bid Delivery
Unlike centralized relays that heavily rely on a single centralized source of truth, such as Redis, for bid propagation, the Relay project takes a distributed approach. We deliver bids to distributed instances, allowing for improved scalability, fault tolerance, and reduced reliance on a single point of failure. This decentralized bid delivery mechanism ensures higher availability and robustness of the relay network.

### Memory-Based Operations
One of the key differentiators of the Relay project is its heavy utilization of memory for serving various operations. Instead of relying on frequent database access, we leverage in-memory storage to serve most of the required data. This approach results in significantly faster response times and reduced latency for bid retrieval, payload decoding, and other essential operations.

### Efficient Data Storage
In contrast to relying solely on a costly PostgreSQL database for storing large volumes of data, the Relay project adopts a more cost-effective approach. We store increasing amounts of data in files, utilizing file-based storage systems. This strategy not only reduces infrastructure costs but also enhances performance and scalability, as file operations can be optimized and scaled more efficiently.

## Distribution

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
