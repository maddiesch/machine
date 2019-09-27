package machine

import (
	"context"
	"sync"
)

var (
	stdlibI *Implementation
	stdlibS sync.Once
)

func stdlib() *Implementation {
	stdlibS.Do(func() {
		i := &Implementation{}

		i.addFunc(true, "set", "returns the passed in value", func(in string) string { return in })

		i.addFunc(true, "setf", "returns the passed in value", func(in float64) float64 { return in })

		i.addFunc(true, "fatal", "throws a runtime error with message as the first argument", func(in string) error {
			return &RuntimeError{
				Code:    "Fatal",
				Message: in,
			}
		})

		i.addFunc(true, "env", "returns the environment variable with the given name", func(ctx context.Context, name string) string {
			st := Mac(ctx)
			if st == nil {
				return ""
			}
			return st.Getenv(name)
		})

		i.addFunc(true, "ret", "returns the groups array of return values", func(ctx context.Context) interface{} {
			return LastReturn(ctx)
		})

		i.freeze()
		stdlibI = i
	})

	return stdlibI
}

func stdlibHasFunc(name string) bool {
	for n := range stdlib().funcs {
		if n == name {
			return true
		}
	}
	return false
}
