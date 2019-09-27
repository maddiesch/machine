package machine_test

import (
	"testing"

	. "github.com/maddiesch/machine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// d, _ := json.MarshalIndent(p, "", "    ")
// fmt.Println(string(d))

// ir, _ := p.IR()
// fmt.Println(base64.StdEncoding.EncodeToString(ir))

func TestCompileSource(t *testing.T) {
	t.Run("given valid source", func(t *testing.T) {
		prog, err := CompileSource(load("example.mac"))

		require.NoError(t, err)

		t.Run("ensure that the generated source will create the same program", func(t *testing.T) {
			p2, err := CompileSource(prog.Source)

			require.NoError(t, err)

			assert.True(t, NodeCompare(prog.Entry, p2.Entry))
		})
	})

	t.Run("compiling a nested function", func(t *testing.T) {
		t.Run("func first", func(t *testing.T) {
			ok := progFromBase64(`ChQKHFMdgKwCBg6zFeZJvYnsTzZSWxIUZm9vKGJhcihmMC45KSBiYXopOwoaowEKFAocUx3iKwqRA0Rq8DdN/xVL3y2fEAEaiAEKFAocUx0snbWH2+7Ig5tTJZ9qwT/KEAMaRgoUChxTHYaAoVZTCBvsKTIRwe70v2oQAxolChQKHFMdGEF/AKghQZ2cOw58SVJknRAEKgsIARnNzMzMzMzsPyoFEgNiYXIaHwoUChxTHZeW30E2I1uG8BLCV54V11sQBCoFEgNiYXoqBRIDZm9vIgcKA2ZvbxABIgcKA2JhchAB`)

			prog, err := CompileSource(`foo(bar(f0.9) baz);`)

			require.NoError(t, err)

			assert.True(t, NodeCompare(prog.Entry, ok.Entry))
		})

		t.Run("func mid", func(t *testing.T) {
			ok := progFromBase64(`ChQKHFG86mRTe8GPCKH/tRpc7XeJBBIbb25lKHR3byB0aHJlZShmb3VyKSBmaXZlKTsKGsIBChQKHFG8yziB45i3U+O5zVhhJpk0hxABGqcBChQKHFG8l5uVknKV3SzWDXpZ2CYeKRADGh8KFAocUbwWy9JApg+7x4QmQScS53kxEAQqBRIDdHdvGkMKFAocUbxLCXmBje/4pbzjpWi9JIQtEAMaIAoUChxRvBATLIfnN8WfaApV/1JIrfQQBCoGEgRmb3VyKgcSBXRocmVlGiAKFAocUby7qPvGc3or8CGUf0D/wJKhEAQqBhIEZml2ZSoFEgNvbmUiBwoDb25lEAEiCQoFdGhyZWUQAQ==`)

			prog, err := CompileSource(`one(two three(four) five);`)

			require.NoError(t, err)

			assert.True(t, NodeCompare(prog.Entry, ok.Entry))
		})

		t.Run("deep nested calls", func(t *testing.T) {
			ok := progFromBase64(`ChQKHFVbeX6/RqXIzyHm7TR6z4BZUBImb25lKHR3byB0aHJlZShmb3VyKCkgZml2ZSBzaXgoc2V2ZW4pKSkahwIKFAocVVufUE3dAY2W4QxX/0eDaytJEAEa7AEKFAocVVvXxN7TvL/jmQLnLfV8sVIBEAMaHwoUChxVW+A4g+WdjiXYC79qz3pDEX4QBCoFEgN0d28aqQEKFAocVVunGovp3Q7ksxiMPT09frLiEAMaIAoUChxVW34Q4d0ieqHE9ywV6cqljpcQAyoGEgRmb3VyGiAKFAocVVuQBpSec1Si9F87ZHDn5ligEAQqBhIEZml2ZRpCChQKHFVblfc81OzvWha70JEr/nnchhADGiEKFAocVVtbZ7vwp44xB5EJ2Q8HdGn6EAQqBxIFc2V2ZW4qBRIDc2l4KgcSBXRocmVlKgUSA29uZSIHCgNzaXgQASIHCgNvbmUQASIJCgV0aHJlZRABIggKBGZvdXIQAQ==`)

			p, err := CompileSource(`one(two three(four() five six(seven)))`)

			require.NoError(t, err)

			assert.True(t, NodeCompare(p.Entry, ok.Entry))
		})
	})

	t.Run("given invalid UTF-8 in source", func(t *testing.T) {
		b := []byte{102, 111, 111, 40, 239, 191, 189, 98, 97, 114, 41}

		_, err := CompileSource(string(b))

		require.Error(t, err)

		_, ok := err.(*SourceError)

		require.True(t, ok)

		assert.Equal(t, "Source error (Ln 1, Col 5): failed to decode UTF-8 character", err.Error())
	})
}
