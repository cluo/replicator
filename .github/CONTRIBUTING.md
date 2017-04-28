# Contributing to Replicator

**First** of all welcome and thank you for even considering to contribute to the replicator project. If you're unsure or afraid of anything, just ask or submit the issue or pull request anyways. You won't be yelled at for giving your best effort.

If you wish to work on Replicator itself or any of its built-in systems, you will first need [Go](https://golang.org/) installed on your machine (version 1.8+ is required) and ensure your [GOPATH](https://golang.org/doc/code.html#GOPATH) is correctly configured.

# Building

Project is linted, tested and built using make:

```
make
```

The resulting binary file will be stored in the project root directory and is named `replicator-local` which can be invoked as required. The binary is built by default for the host system only. You can cross-compile and build binaries for a number of different systems and architectures by invoking the build script:

```
./scripts/build.sh
```

The build script outputs the binary files to `/pkg`:

```
darwin-386-replicator
darwin-amd64-replicator
freebsd-386-replicator
freebsd-amd64-replicator
freebsd-arm-replicator
linux-386-replicator
linux-amd64-replicator
linux-arm-replicator
```

See [docs](https://golang.org/doc/install/source) for the whole list of available `GOOS` and `GOARCH`
values.
