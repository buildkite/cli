go-cursor
=========

Provides [ANSI escape sequences](http://en.wikipedia.org/wiki/ANSI_escape_code)
(strings) for manipulating cursor on terminals.

Usage
-----

Here is equivalent of `clear` program (`cls` command on Windows) written in Go
using `go-cursor`:

```go
package main

import (
    "fmt"
    "github.com/ahmetalpbalkan/go-cursor"
)

func main() {
    fmt.Print(cursor.ClearEntireScreen())
    fmt.Print(cursor.MoveTo(0, 0))
}
```
