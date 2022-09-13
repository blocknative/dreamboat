

# 🛳️ Dreamboat 

[![GoDoc](https://godoc.org/github.com/blocknative/dreamboat?status.svg)](https://godoc.org/github.com/blocknative/dreamboat)
[![Go Report Card](https://goreportcard.com/badge/github.com/blocknative/dreamboat?style=flat-square)](https://goreportcard.com/report/github.com/blocknative/dreamboat)
<!-- [![Go](https://github.com/blocknative/dreamboat/actions/workflows/go.yml/badge.svg)](https://github.com/blocknative/dreamboat/actions/workflows/go.yml) -->

An Ethereum 2.0 Relay with proposer-builder separation (PBS) and MEV-boost.  Commissioned, built, and put to sea by Blocknative.

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

## What is Dreamboat?

Dreamboat is a relay for Ethereum 2.0, meaning it ships blocks from builders to validators.  Dreamboat is for anyone who wants to strengthen the Eth 2.0 community by running their own relay.  Dreamboat's all-in-one design, modular code base and stunning performance will dazzle beginners and seasoned hackers alike.  All aboard!

More precisely, Dreamboat is a spec-compliant [builder](https://github.com/ethereum/builder-specs) and [relay](https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5) that allows block-builders and validators to interface with eachother through [MEV-boost](https://github.com/flashbots/mev-boost).  

Dreamboat will vanish, as if a dream, once in-protocol PBS is live.  Until then, it sails the ephemeral seas of block space, delivering its trusted cargo of blocks to validators, guided simply by a lighthouse.

## Design Goals

Special thanks to Flashbots for their commiment to open source. This based on Flashobot's [boost-geth-builder](https://github.com/flashbots/boost-geth-builder) and [mev-boost-relay](https://github.com/flashbots/mev-boost-relay).  The two codebases are expected to diverge significantly over time, as we continue to work on Dreamboat's core design goals, which are:

1. **Extensibility.**  Dreamboat provides well-factored interfaces that make it easy to change its behavior.  We use [Go Datastore](https://pkg.go.dev/github.com/ipfs/go-datastore) for its persistence layer, so you can swap out the default BadgerDB store with your favorite database, and provide a clean separation between core relay logic, the beacon-chain client, and the HTTP server.
2. **Reliability.**  Dreamboat is designed to automatically recover from failures and degrade gracefully in the presence of partial outages.  Work is ongoing implement a [supervision-tree architecture](https://ferd.ca/the-zen-of-erlang.html) in Dreamboat, so that you never miss a slot.
3. **Performance.** We have put significant engineering effort into tuning Dreamboat's performance, and have much, much more in store.  The HTTP server's hot paths are 100% lock-free, concurrency is carefully tuned, and there are ongoing efforts to optimize the garbage collector and speed up signature validation.  The result is a ship that doesn't just sail... it *glides*.
4. **Transparency.**  Dreamboat implements the [Relay Data API](https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#38a21c8a40e64970904500eb7b373ea5), so you can audit past proposals.  But we take a much broader view of transparency, which includes **code transparency**.  A major cleanup effort is underway to make -- and *keep* -- Dreamboat's code *legible*.  We believe this is an essential part of ensuring transparency and reliability.

We are continuously improving Dreaboat's runtime performance, standards compliance, reliability and transparency.

## Planned Features & Enhancements

The following features and enhancements are in-progress.

- [x] Support for multiple storage backends (Redis, Postgres, BadgerDB, Filesystem, etc.)
- [x] Runtime profiling & tracing endpoint
- [x] Parallel validator-signature verification
- [x] Contention-free execution along hot paths
- [ ] Support for multiple external block builders
- [ ] Support for multiple beacon clients

## Support

Stuck?  Don't despair!  Drop by our [discord](https://discord.com/invite/KZaBVME) to ask for help, or just to say hello. We're friendly! 👋


## Developing

### Building Locally

```
$ go install cmd/relay
```


### Architecture

Proposer Builder Separation is a solution to shield Validators from the centralizing force the black hole of MEV has released. The next chapter in this saga is to protect builders from too extreme of centralizing forces.

<img width="874" alt="PBS Context Diagram" src="https://user-images.githubusercontent.com/22778355/188292682-421dee2e-b11c-46ca-af08-7f6c0e0d665e.png">

## API Compatibility

Dreamboat follows semantic versioning.  Changes to the REST API are governed by the Flashbots specifications.

The `main` breanch is **NOT** guaranteed to be stable.  Use the latest git tag in production!
