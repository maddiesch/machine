package machine

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// Implementation contains the implementation of a machine
type Implementation struct {
	mu     sync.RWMutex
	funcs  map[string]*iFunc
	frozen bool
	stdInj bool
}

// Func adds a function handler to the implementation
func (i *Implementation) Func(name string, handler interface{}) {
	if stdlibHasFunc(name) {
		panic(fmt.Errorf("attempting to override as stdlib function is not allowed '%s'", name))
	}

	i.addFunc(false, name, "", handler)
}

// Functions returns a list of function documentation.
func (i *Implementation) Functions() []string {
	i.mergeStdlib()

	i.mu.RLock()
	defer i.mu.RUnlock()

	str := make([]string, 0)
	for _, f := range i.funcs {
		str = append(str, f.syntax())
	}

	sort.Strings(str)

	return str
}

func (i *Implementation) addFunc(std bool, name string, desc string, handler interface{}) {
	if i.isFrozen() {
		panic(fmt.Errorf("implementation is frozen, unable to modify"))
	}

	tp := reflect.TypeOf(handler)
	if tp.Kind() != reflect.Func {
		panic(fmt.Errorf("attempting to add a function '%s' without a function handler", name))
	}

	fn := &iFunc{
		name:  name,
		desc:  desc,
		impl:  handler,
		std:   std,
		tp:    tp,
		recC:  tp.NumIn(),
		rRecC: tp.NumIn(),
		retC:  tp.NumOut(),
		rRetC: tp.NumOut(),
	}

	if tp.NumIn() > 0 {
		fpr := tp.In(0)

		if fpr.Implements(contextType) {
			fn.recC--
			fn.recCxt = true
		}
	}

	switch tp.NumOut() {
	case 0:
	case 1:
		out := tp.Out(0)
		if out.Implements(errorType) {
			fn.retErr = true
			fn.retC--
		}
	case 2:
		out := tp.Out(1)
		if out.Implements(errorType) {
			fn.retErr = true
			fn.retC--
		} else {
			panic(fmt.Errorf("function %s's second return value must be an error", name))
		}
	default:
		panic(fmt.Errorf("function for '%s' can't return more than 2 arguments", name))
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.funcs == nil {
		i.funcs = make(map[string]*iFunc, 0)
	}
	if _, ok := i.funcs[name]; ok {
		panic(fmt.Errorf("attempting to redefine a function with name '%s'", name))
	}
	i.funcs[name] = fn
}

type lookupFunc func(string) (*iFunc, error)

func (i *Implementation) lookup(name string) (*iFunc, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i, ok := i.funcs[name]; ok {
		return i, nil
	}
	return nil, &RuntimeError{
		Code:    "FuncNotFound",
		Message: fmt.Sprintf("function with name '%s' not found", name),
	}
}

func (i *Implementation) dup() *Implementation {
	i.mu.Lock()
	defer i.mu.Unlock()
	funcs := make(map[string]*iFunc, len(i.funcs))
	for name, f := range i.funcs {
		n := iFunc(*f)
		funcs[name] = &n
	}
	return &Implementation{
		funcs:  funcs,
		frozen: true,
		stdInj: i.stdInj,
	}
}

func (i *Implementation) mergeStdlib() {
	if i.hasStdlib() {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.funcs == nil {
		i.funcs = make(map[string]*iFunc, 0)
	}

	std := stdlib()

	for name, f := range std.funcs {
		n := iFunc(*f)
		i.funcs[name] = &n
	}

	i.stdInj = true
}

func (i *Implementation) freeze() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.frozen = true
}

func (i *Implementation) isFrozen() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.frozen
}

func (i *Implementation) hasStdlib() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.stdInj
}

type iFunc struct {
	name   string
	desc   string
	impl   interface{}
	std    bool
	retErr bool
	recCxt bool
	recC   int
	rRecC  int
	retC   int
	rRetC  int
	tp     reflect.Type
}

func (fn *iFunc) call(ctx context.Context, args []reflect.Value) (reflect.Value, error) {
	if len(args) != fn.recC {
		return reflect.Value{}, &RuntimeError{
			Code:    "ArgumentError",
			Message: fmt.Sprintf("Attempting to call '%s' with %d arguments. Expected %d", fn.name, len(args), fn.recC),
		}
	}

	fnV := reflect.ValueOf(fn.impl)

	if fn.recCxt {
		args = append([]reflect.Value{reflect.ValueOf(ctx)}, args...)
	}

	out := fnV.Call(args)

	// Nothing to return
	if fn.retC == 0 && !fn.retErr {
		return reflect.Value{}, nil
	}

	// We only return an error and no value
	if fn.retC == 0 && fn.retErr && len(out) == 1 {
		if !out[0].IsNil() && out[0].Type().ConvertibleTo(errorType) {
			return reflect.Value{}, out[0].Interface().(error)
		}
		return reflect.Value{}, nil
	}

	// We only return a value
	if fn.retC == 1 && !fn.retErr && len(out) == 1 {
		return out[0], nil
	}

	// We return both a value and an error
	if fn.retC == 1 && fn.retErr && len(out) == 2 {
		if !out[1].IsNil() && out[1].Type().ConvertibleTo(errorType) {
			return out[0], out[1].Interface().(error)
		}
		return out[0], nil
	}

	return reflect.Value{}, &RuntimeError{
		Code:    "ResultError",
		Message: fmt.Sprintf("Func '%s'", fn.name),
	}
}

func (fn *iFunc) syntax() string {
	b := strings.Builder{}

	b.WriteString(fn.name)
	b.WriteRune('(')

	{ // Arguments
		for i := 0; i < fn.rRecC; i++ {
			if fn.recCxt && i == 0 {
				continue
			}

			in := fn.tp.In(i)

			b.WriteString(in.String())

			if i != fn.rRecC-1 {
				b.WriteRune(' ')
			}
		}
	}

	b.WriteRune(')')

	if fn.retC == 1 {
		out := fn.tp.Out(0)

		b.WriteRune(' ')
		b.WriteString(out.String())
	}

	b.WriteRune(';')

	return b.String()
}

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

	errorType = reflect.TypeOf((*error)(nil)).Elem()
)
