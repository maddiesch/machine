package machine

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/maddiesch/failable"
	"github.com/segmentio/ksuid"
)

// SyntaxError is thrown when the parser encounters a token that it is not expecting.
type SyntaxError struct {
	Token   *TokenIL
	Message string
	Node    *NodeIL
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("Syntax Error (Ln %d, Col %d, <%s>): %s", e.Token.Line, e.Token.Column, e.Node.Kind.String(), e.Message)
}

func parser(ctx context.Context, comp *compiler, fail failable.FailFunc) {
	tokens := make([]*TokenIL, len(comp.Tokens))
	copy(tokens, comp.Tokens) // If we don't perform the copy, the tokens slice gets mangled in the parser

	node := newNode(NodeIL_ROOT)

	for i := 0; i < len(tokens); {
		in := parseTokenInput{
			compiler: comp,
			node:     node,
			token:    tokens[i],
			before:   tokens[:i],
			after:    tokens[i+1:],
			depth:    0,
		}

		c, _ := parseToken(ctx, fail, in)

		i += c
	}

	comp.Ast = node
}

type parseTokenInput struct {
	compiler *compiler
	node     *NodeIL
	token    *TokenIL
	before   []*TokenIL
	after    []*TokenIL
	depth    int
}

func (i *parseTokenInput) prev() (*TokenIL, bool) {
	if len(i.before) == 0 {
		return nil, false
	}
	return i.before[len(i.before)-1], true
}

func (i *parseTokenInput) next() (*TokenIL, bool) {
	if len(i.after) == 0 {
		return nil, false
	}
	return i.after[0], true
}

func (i *parseTokenInput) nextIsProbablyFunc() bool {
	if len(i.after) < 2 {
		return false
	}
	return i.after[0].Kind == TokenIL_VALUE && i.after[1].Kind == TokenIL_OPEN
}

func (i *parseTokenInput) nextIsProbablyGroup() bool {
	if len(i.after) < 3 {
		return false
	}
	return i.after[0].Kind == TokenIL_OPEN && i.after[1].Kind == TokenIL_VALUE && i.after[2].Kind == TokenIL_OPEN
}

func (i *parseTokenInput) syntax(m string) *SyntaxError {
	return &SyntaxError{
		Token:   i.token,
		Node:    i.node,
		Message: m,
	}
}

// Takes the current node, the current token, all the previous tokens, all the remaining tokens, and a failing function.
//
// Returns the number of tokens consumed, and if the the calling node should close.
func parseToken(ctx context.Context, fail failable.FailFunc, input parseTokenInput) (int, bool) {
	switch input.token.Kind {
	case TokenIL_VALUE:
		return parseValueToken(ctx, input, fail)
	case TokenIL_OPEN:
		return parseOpenToken(ctx, input, fail)
	case TokenIL_END:
		return 1, true
	case TokenIL_CLOSE:
		return parseCloseToken(ctx, input, fail)
	case TokenIL_DOT:
		return parseDotToken(ctx, input, fail)
	case TokenIL_PIPE:
		return parsePipeToken(ctx, input, fail)
	case TokenIL_ASSIGN:
		return parseAssignToken(ctx, input, fail)
	case TokenIL_VAR:
		return parseVarToken(ctx, input, fail)
	default:
		panic(fmt.Sprintf("Unknown token: %+v", input.token))
	}
}

func parseDotToken(ctx context.Context, in parseTokenInput, fail failable.FailFunc) (int, bool) {
	if len(in.node.Children) == 0 {
		fail(in.syntax("Unexpected chain. The node has no children to chain from."))
	}

	last := in.node.Children[len(in.node.Children)-1]

	switch last.Kind {
	case NodeIL_GROUP, NodeIL_FUNC:
		if !in.nextIsProbablyFunc() && !in.nextIsProbablyGroup() {
			fail(in.syntax("Unexpected chain. The next call doesn't appear to be a function or group."))
		}

		new := newNode(NodeIL_ROOT)

		consumed := 1

		for i := 0; i < len(in.after); {
			in := parseTokenInput{
				compiler: in.compiler,
				node:     new,
				token:    in.after[i],
				before:   append(in.before, in.after[:i]...),
				after:    in.after[i+1:],
				depth:    (in.depth + 1),
			}

			c, d := parseToken(ctx, fail, in)

			consumed += c
			i += c

			if d {
				break
			}
		}

		if len(new.Children) != 1 {
			fail(in.syntax(fmt.Sprintf("Failed to chain. Only expected one root child. Got %d", len(new.Children))))
		}

		last.Chained = new.Children[0]

		return consumed, true
	case NodeIL_VALUE:
		next, ok := in.next()
		if !ok || next.Kind != TokenIL_VALUE {
			fail(in.syntax(fmt.Sprintf("Expected to find a value after .")))
		}

		if strings.HasPrefix(last.Value.Str, "f") {
			raw := fmt.Sprintf("%s.%s", last.Value.Str[1:], next.Value)
			flt, err := strconv.ParseFloat(raw, 64)

			if err != nil {
				fail(in.syntax(fmt.Sprintf("Invalid float value: %v", err)))
			}

			last.setValue(flt)

			return 2, false
		}

		fail(in.syntax(fmt.Sprintf("Unabled to determine the value for %+v.%s", last.Value, next.Value)))
	default:
		fail(in.syntax("Unexpected chain. You can only chain from a group or a function."))
	}

	panic("WTF... this should never happen")
}

func parseAssignToken(ctx context.Context, in parseTokenInput, fail failable.FailFunc) (int, bool) {
	if len(in.before) < 2 {
		fail(in.syntax("Unexpected assignment. Expected a value and definition before."))
	}
	if len(in.after) == 0 {
		fail(in.syntax("Unexpected assignment. Nothing to assign to the variable."))
	}

	kind := in.before[len(in.before)-2]
	name := in.before[len(in.before)-1]

	if kind.Kind != TokenIL_VALUE || kind.Value != "const" {
		fail(&SyntaxError{
			Token:   kind,
			Node:    in.node,
			Message: "The leading assignment is not a valid type.",
		})
	}

	if name.Kind != TokenIL_VALUE {
		fail(&SyntaxError{
			Token:   name,
			Node:    in.node,
			Message: "The leading assignment name is not a valid type.",
		})
	}

	new := newNode(NodeIL_ASSIGN)
	new.SubType = kind.Value
	new.setValue(name.Value)

	root := newNode(NodeIL_ROOT)

	consumed := 1

	for i := 0; i < len(in.after); {
		in := parseTokenInput{
			compiler: in.compiler,
			node:     root,
			token:    in.after[i],
			before:   append(in.before, in.after[:i]...),
			after:    in.after[i+1:],
			depth:    (in.depth + 1),
		}

		c, d := parseToken(ctx, fail, in)

		consumed += c
		i += c

		if d {
			break
		}
	}

	if len(root.Children) != 1 {
		fail(in.syntax(fmt.Sprintf("Failed to chain. Only expected one root child. Got %d", len(root.Children))))
	}

	new.Chained = root.Children[0]

	in.node.addChild(new)

	return consumed, true
}

func parseOpenToken(ctx context.Context, in parseTokenInput, fail failable.FailFunc) (int, bool) {
	if _, ok := in.next(); !ok {
		fail(in.syntax("Unexpected Open. Found EOF, expected something."))
	}

	node := in.node

	switch node.Kind {
	case NodeIL_FUNC:
		fallthrough
	case NodeIL_ROOT, NodeIL_GROUP:
		var new *NodeIL

		if prev, ok := in.prev(); !ok || prev.Kind != TokenIL_VALUE {
			new = newNode(NodeIL_GROUP)
		} else {
			prev, _ := in.prev()
			if contains(nativeFunctionNames, prev.Value) {
				new = newNode(NodeIL_NAT)
			} else {
				new = newNode(NodeIL_FUNC)
				in.compiler.FuncCalls[prev.Value]++
			}
			new.setValue(prev.Value)
		}

		node.addChild(new)

		consumed := 1

		for i := 0; i < len(in.after); {
			in := parseTokenInput{
				compiler: in.compiler,
				node:     new,
				token:    in.after[i],
				before:   append(in.before, in.after[:i]...),
				after:    in.after[i+1:],
				depth:    (in.depth + 1),
			}

			c, d := parseToken(ctx, fail, in)

			consumed += c
			i += c

			if d {
				break
			}
		}

		return consumed, false
	default:
		fail(in.syntax("Unexpected Open. The open is not not in a valid context."))
	}

	panic("WTF... this should never happen")
}

func parseCloseToken(_ context.Context, in parseTokenInput, fail failable.FailFunc) (int, bool) {
	switch in.node.Kind {
	case NodeIL_GROUP, NodeIL_FUNC, NodeIL_NAT:
		return 1, true
	default:
		fail(in.syntax("Unexpected close. The thing you're attempting to close can't be closed."))
	}

	panic("WTF... this should never happen")
}

func parsePipeToken(_ context.Context, in parseTokenInput, fail failable.FailFunc) (int, bool) {
	if in.node.Kind != NodeIL_GROUP {
		fail(in.syntax("Unexpected Pipe. You can't pipe outside of a group."))
	}
	if !in.nextIsProbablyFunc() {
		fail(in.syntax("Unexpected Pipe. The next call doesn't appear to be a function."))
	}

	return 1, false
}

func parseValueToken(_ context.Context, in parseTokenInput, fail failable.FailFunc) (int, bool) {
	if in.node.Kind != NodeIL_FUNC && in.node.Kind != NodeIL_NAT {
		return 1, false // Something else will backtrack and consume this soon.
	}

	if next, ok := in.next(); ok && next.Kind == TokenIL_OPEN {
		return 1, false // This is probably a nested function call.
	}

	new := newNode(NodeIL_VALUE)
	switch in.token.Value {
	case "true":
		new.setValue(true)
	case "false":
		new.setValue(false)
	default:
		new.setValue(in.token.Value)
	}

	in.node.addChild(new)

	return 1, false
}

func parseVarToken(_ context.Context, in parseTokenInput, fail failable.FailFunc) (int, bool) {
	next, ok := in.next()

	if !ok {
		fail(in.syntax("Unexpected variable. Found EOF, expected a name."))
	}

	if next.Kind != TokenIL_VALUE {
		fail(&SyntaxError{
			Token:   next,
			Message: "Unexpected variable fetch. Expected a name.",
			Node:    in.node,
		})
	}

	if in.node.Kind != NodeIL_FUNC {
		fail(in.syntax("Attempting to use a variable outside a function call."))
	}

	new := newNode(NodeIL_VAR)
	new.setValue(next.Value)

	in.node.addChild(new)

	return 2, false
}

func newNode(k NodeIL_Kind) *NodeIL {
	return &NodeIL{
		Id:       ksuid.New().Bytes(),
		Kind:     k,
		Children: make([]*NodeIL, 0),
	}
}

func (n *NodeIL) addChild(child ...*NodeIL) {
	for _, c := range child {
		n.Children = append(n.Children, c)
	}
}

func (n *NodeIL) setValue(v interface{}) {
	switch v := v.(type) {
	case string:
		n.Value = &NodeIL_DValue{Str: v, Kind: NodeIL_DValue_STR}
	case float64:
		n.Value = &NodeIL_DValue{Flt: v, Kind: NodeIL_DValue_FLT}
	case bool:
		n.Value = &NodeIL_DValue{Bool: v, Kind: NodeIL_DValue_BOOL}
	default:
		panic(fmt.Sprintf("Attempting to set an invalid value type %+v", v))
	}
}
