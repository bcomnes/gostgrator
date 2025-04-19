# gostgrator
[![Actions Status][action-img]][action-url]
[![SocketDev][socket-image]][socket-url]
[![PkgGoDev][pkg-go-dev-img]][pkg-go-dev-url]

[action-img]: https://github.com/bcomnes/gostgrator/actions/workflows/test.yml/badge.svg
[action-url]: https://github.com/bcomnes/gostgrator/actions/workflows/test.yml
[pkg-go-dev-img]: https://pkg.go.dev/badge/github.com/bcomnes/gostgrator
[pkg-go-dev-url]: https://pkg.go.dev/github.com/bcomnes/gostgrator
[socket-image]: https://socket.dev/api/badge/go/package/github.com/bcomnes/gostgrator?version=v1.0.2
[socket-url]: https://socket.dev/go/package/github.com/bcomnes/gostgrator?version=v1.0.2

A migration tool for Postgres and SQLite. A port of [rickbergfalk/postgrator](https://github.com/rickbergfalk/postgrator) to go. 

## Install

```console
go get github.com/bcomnes/gostgrator
```

## Usage

``` go
package main

import (
  "github.com/bcomnes/gostgrator"
)

func main() {
  fmt.Println("hello world")
}
```

See more examples on [PkgGoDev][pkg-go-dev-url].

## API

See API docs on [PkgGoDev][pkg-go-dev-url].

## License

MIT
