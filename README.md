## Service Test Tool

### Design considerations

The tool is designed according with following considerations:
- require no installation of the tool on the dev machine or on the host where it is to be run
- require no setup of the building toolchain to build the tool
- single small size binary file
- should run requests concurrently

## Using

To print help:

```
./service-test -h
```

The tool can be copied to a Pod in a K8s cluster, to be run inside the cluster, using the 
`kubectl cp` command.

## Building the tool

The tool can be built either with installed Golang toolchain or using a Golang Docker container.

### Building with installed toolchain

```
go build -o bin/service-test
```

### Building using Docker container

```
docker run --rm -v "$PWD":/files -w /files golang:1.17 go build -o bin/service-test
```

### Cross-compilation

In order to build the tool in one system (e.g. MacOS) to be run on another system (e.g. Linux), 
the target system needs to be set in the `GOOS` environment variable.

On Mac that can be accomplished this way:

```
GOOS=linux go build -o bin/service-test
```
