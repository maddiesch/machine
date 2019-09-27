package machine_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/maddiesch/machine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func load(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)

	root := filepath.Dir(f)

	path := filepath.Join(append([]string{root}, parts...)...)

	b, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	return string(b)
}

func progFromBase64(str string) *ProgramIL {
	p := &ProgramIL{}

	dat, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		panic(err)
	}

	err = p.LoadIR(dat)
	if err != nil {
		panic(err)
	}

	return p
}

func impl() *Implementation {
	i := &Implementation{}

	i.Func("alert", func(metric string, operator string, value string) (string, error) {
		return fmt.Sprintf("alert(%s, %s, %s)", metric, operator, value), nil
	})

	i.Func("warn", func(metric string, operator string, value string) (string, error) {
		return fmt.Sprintf("warn(%s, %s, %s)", metric, operator, value), nil
	})

	i.Func("recover", func(ctx context.Context, operator string, value string) error {
		return nil
	})

	i.Func("page", func(ctx context.Context) {
	})

	i.Func("scale-up", func(appName string, metric string, operator string, value float64) error {
		return nil
	})

	i.Func("scale-down", func(appName string, metric string, operator string, value float64) error {
		return nil
	})

	i.Func("slack", func(channelName string, resourceID string) error {
		return nil
	})

	i.Func("enable", func(kind string, appName string, enabled bool) error {
		return nil
	})

	return i
}

func TestMachine(t *testing.T) {
	t.Run("sanity check", func(t *testing.T) {
		m := New(impl())
		m.Setenv("app-id", "testing-app")

		defer m.Shutdown()

		prog, err := CompileSource(load("example.mac"))

		require.NoError(t, err)

		err = m.Execute(prog)

		assert.NoError(t, err)
	})

	t.Run("given a missing function name, the program fails before executing", func(t *testing.T) {
		i := &Implementation{}

		i.Func("foo", func() {
			t.Log("foo called before the program failed with a missing function name")
			t.Fail()
		})

		prog, err := CompileSource(`foo().not-a-valid-function()`)

		require.NoError(t, err)

		m := New(i)
		defer m.Shutdown()

		err = m.Execute(prog)

		assert.Error(t, err)
	})

	t.Run("variable assignment", func(t *testing.T) {
		t.Run("given an existing variable", func(t *testing.T) {
			i := &Implementation{}

			err := Run(i, `
				const a = env(none);

				const a = env(none);
			`)

			require.Error(t, err)

			assert.Equal(t, "Runtime Error: <AssignmentError> Attempting to reassign a value to a constant.", err.Error())
		})

		t.Run("given an existing but deleted variable", func(t *testing.T) {
			i := &Implementation{}

			err := Run(i, `
				const a = env(none);

				_delete(a);

				const a = env(none);
			`)

			assert.NoError(t, err)
		})
	})
}
