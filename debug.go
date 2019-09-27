// +build debug

package machine

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

func dump(a ...interface{}) {
	_, f, l, _ := runtime.Caller(1)

	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("%s:%d\n", f, l)
	spew.Dump(a...)
}
