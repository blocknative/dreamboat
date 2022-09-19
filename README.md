

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

More precisely, Dreamboat is a spec-compliant [builder](https://github.com/ethereum/builder-specs) and [relay](https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5) that allows block-builders and validators to interface with eachother through [MEV-boost](https://github.com/flashbots/mev-boost).  

Eventually Dreamboat will vanish, as if a dream, once in-protocol PBS is live.  Until then, it sails the ephemeral seas of block space, delivering its trusted cargo of blocks to validators, guided simply by a lighthouse.

## Design Goals

Special thanks to Flashbots for their commiment to open source. This project was orginally based on the relay in Flashobot's [boost-geth-builder](https://github.com/flashbots/boost-geth-builder) and [mev-boost-relay](https://github.com/flashbots/mev-boost-relay).  We hope to continue to diverge in implementation over time to help strengthen #RelayDiversity as we continue to work on Dreamboat's core design goals:

1. **Extensibility.**  Dreamboat provides well-factored interfaces that make it easy to change its behavior.  We use [Go Datastore](https://pkg.go.dev/github.com/ipfs/go-datastore) for its persistence layer, so you can swap out the default [BadgerDB](https://github.com/dgraph-io/badger) store with your favorite database, and provide a clean separation between core relay logic, the beacon-chain client, and the HTTP server.
2. **Reliability.** Work is ongoing implement a [supervision-tree architecture](https://ferd.ca/the-zen-of-erlang.html) in Dreamboat, so that you never miss a block.
3. **Performance.** We have put significant engineering effort into tuning Dreamboat's performance, and have much, much more in store.  The HTTP server's hot paths are 100% lock-free, concurrency is carefully tuned, and there are ongoing efforts to optimize the garbage collector and speed up signature validation.  The result is a ship that doesn't just sail... it *glides*.
4. **Transparency.**  Dreamboat implements the [Relay Data API](https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#38a21c8a40e64970904500eb7b373ea5), so you can audit past proposals.  But we take a much broader view of transparency, which includes **code transparency**.  A major cleanup effort is underway to make -- and *keep* -- Dreamboat's code *legible*.  We believe this is an essential part of ensuring transparency and reliability.

We are continuously improving Dreaboat's runtime performance, standards compliance, reliability and transparency.

## Planned Features & Enhancements

The following features and enhancements are in-progress.

- [x] Support for multiple storage backends (Redis, Postgres, BadgerDB, Filesystem, etc.)
- [x] Runtime profiling & tracing endpoint
- [x] Parallel validator-signature verification
- [x] Contention-free execution along hot paths
- [ ] Support for multiple beacon clients (in testing stage)
- [ ] Support for multiple external block builders


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
