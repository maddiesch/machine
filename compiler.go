package machine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	proto "github.com/golang/protobuf/proto"
	"github.com/maddiesch/failable"
	"github.com/segmentio/ksuid"
)

type compiler struct {
	Source    string
	Hash      []byte
	Tokens    []*TokenIL
	Ast       *NodeIL
	FuncCalls map[string]uint64
}

// CompileSource takes source code and turns it into a machine program.
func CompileSource(src string) (*ProgramIL, error) {
	ctx := context.Background()

	hash := sha256.Sum256([]byte(src))

	comp := &compiler{
		Source:    src,
		Hash:      hash[:],
		Tokens:    []*TokenIL{},
		FuncCalls: map[string]uint64{},
	}

	err := failable.DoWithContext(ctx, func(ctx context.Context, fail failable.FailFunc) {
		tokenize(ctx, comp, fail)
	})
	if err != nil {
		return nil, err
	}

	err = failable.DoWithContext(ctx, func(ctx context.Context, fail failable.FailFunc) {
		parser(ctx, comp, fail)
	})
	if err != nil {
		return nil, err
	}

	return &ProgramIL{
		Id:        ksuid.New().Bytes(),
		Source:    comp.GenerateSource(),
		Entry:     comp.Ast,
		FuncCalls: comp.FuncCalls,
	}, nil
}

// GenerateSource returns source code generated from the tokens.
func (c *compiler) GenerateSource() string {
	builder := strings.Builder{}

	for i, token := range c.Tokens {
		switch token.Kind {
		case TokenIL_NONE:
			panic("lol... this should never happen")
		case TokenIL_VALUE:
			builder.WriteString(token.Value)
			if len(c.Tokens)-1 > i {
				switch c.Tokens[i+1].Kind {
				case TokenIL_VALUE, TokenIL_VAR:
					builder.WriteRune(' ')
				default:
					// Do nothing
				}
			}
		case TokenIL_OPEN:
			builder.WriteRune('(')
		case TokenIL_CLOSE:
			builder.WriteRune(')')
			if len(c.Tokens)-1 > i {
				switch c.Tokens[i+1].Kind {
				case TokenIL_VALUE, TokenIL_VAR:
					builder.WriteRune(' ')
				default:
					// Do nothing
				}
			}
		case TokenIL_END:
			builder.WriteRune(';')
			builder.WriteRune('\n')
		case TokenIL_DOT:
			builder.WriteRune('.')
		case TokenIL_PIPE:
			builder.WriteRune('|')
		case TokenIL_ASSIGN:
			builder.WriteString(" = ")
		case TokenIL_VAR:
			builder.WriteRune('$')
		default:
			panic(fmt.Sprintf("Hey... dummy... don't forget about token: %+v", token))
		}
	}

	return builder.String()
}

func (c *compiler) scanner() *bufio.Scanner {
	return bufio.NewScanner(strings.NewReader(c.Source))
}

// IR returns the Intermediate Language Representation of the program
func (p *ProgramIL) IR() ([]byte, error) {
	return proto.Marshal(p)
}

// LoadIR re-creates the program from IR
func (p *ProgramIL) LoadIR(ir []byte) error {
	return proto.Unmarshal(ir, p)
}
