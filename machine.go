package machine

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"time"
)

// Machine is the VM that will run a program.
type Machine struct {
	impl      *Implementation
	mu        sync.RWMutex
	exec      chan *mProcess
	count     uint64
	lastState *machineST
	env       map[string]string
}

// MacC is the interface available in a running program's context.
type MacC interface {
	Getenv(string) string
}

// A machine process that is waiting to be run.
type mProcess struct {
	prog *ProgramIL
	done chan interface{}
	in   time.Time
}

// New returns a new machine.
func New(impl *Implementation) *Machine {
	impl.mergeStdlib()
	i := impl.dup()

	m := &Machine{
		impl: i,
		exec: make(chan *mProcess),
		env:  make(map[string]string, 0),
	}

	go m.run()

	return m
}

// Getenv returns the environment variable
func (m *Machine) Getenv(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.env[name]
}

// Setenv sets an environment variable
func (m *Machine) Setenv(name string, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.env[name] = value
}

// Shutdown stops the machine.
func (m *Machine) Shutdown() {
	m.exec <- nil
}

// Runs the program from the single threaded execution channel.
func (m *Machine) run() {
	for {
		select {
		case p := <-m.exec:
			m.runPro(p)
		}
	}
}

// Execute runs the program in the machine.
func (m *Machine) Execute(p *ProgramIL) error {
	for name := range p.FuncCalls {
		_, err := m.impl.lookup(name)
		if err != nil {
			return err
		}
	}

	pro := &mProcess{
		prog: p,
		done: make(chan interface{}),
		in:   time.Now(),
	}

	m.exec <- pro

	out := <-pro.done

	switch out := out.(type) {
	case error:
		return out
	default:
		return nil
	}
}

// Actually calls the execution method
func (m *Machine) runPro(p *mProcess) {
	if p == nil { // Stopping the machine will push a nil pending process.
		runtime.Goexit()
	}

	err := m.execute(p.prog)

	if err != nil {
		p.done <- err
	} else {
		p.done <- true
	}
}

// Performs the execution of the program
func (m *Machine) execute(p *ProgramIL) error {
	m.mu.Lock() // lock around resetting the state of the machine
	m.lastState = nil

	// Make a copy of the machine's environment
	env := make(map[string]string, len(m.env))
	for k, v := range m.env {
		env[k] = v
	}

	m.mu.Unlock()

	// Setup the initial state
	s := &machineST{
		lookup: m.impl.lookup,
		ptr:    uintptr(0x10000000),
		progID: p.Id,
		heap:   make(macFrame, 0),
		stack:  make([]macFrame, 0),
		env:    env,
		names:  make(map[string]uintptr, 0),
	}

	// Setup the context
	ctx := context.WithValue(context.Background(), macCtxCurKey, s)

	// Call the entry node. This will be a "ROOT" and will process all of this children.
	_, err := p.Entry.call(ctx, s)

	// Lock around the state
	// Even if we got an error we still "executed" a program.
	m.mu.Lock()
	m.count++
	m.lastState = s
	m.mu.Unlock()

	if err != nil {
		return err
	}

	return nil
}

// Contains the current execution state of the machine.
type machineST struct {
	// The function used to lookup a function by it's name
	lookup lookupFunc

	// The program ID
	progID []byte

	// The current pointer
	ptr uintptr

	// The heap of long lived values.
	heap macFrame

	// The stack. Every node call gets it's own stack
	stack []macFrame

	// A copy of the machine environment
	env map[string]string

	// The table of variable names and the heap pointer for that variable
	names map[string]uintptr
}

// Pushes a new stack frame
func (m *machineST) push() {
	m.stack = append([]macFrame{macFrame{}}, m.stack...)
}

// returns a value from the stack walking up the frames as needed.
func (m *machineST) sGet(ptr uintptr) (reflect.Value, bool) {
	for _, stack := range m.stack {
		if v, ok := stack[ptr]; ok {
			return v, true
		}
	}
	return reflect.ValueOf(nil), false
}

// set a value into the current stack frame
func (m *machineST) sSet(ptr uintptr, v reflect.Value) {
	m.stack[0][ptr] = v
}

// remove the current stack frame and return it.
func (m *machineST) pop() macFrame {
	current := m.stack[0]

	m.stack = m.stack[1:]

	return current
}

// Allow a caller to get an env variable
func (m *machineST) Getenv(name string) string {
	return m.env[name]
}

// The maximum depth of the stack
const maxStackLevel = 2000

var (
	// The stack pointer value for the return value
	stackReturnPtr = uintptr(0xff034680)

	// The node that owns the stack frame
	stackNodeDescPtr = uintptr(0xff00117f)

	// The id of the node that owns the stack frame
	stackNodeIDPtr = uintptr(0xff001180)
)

// A single frame
type macFrame map[uintptr]reflect.Value

// executes a single node and it's children
func (n *NodeIL) call(ctx context.Context, m *machineST) (macFrame, error) {
	m.push() // Start a new stack

	// Checking the stack level.
	if len(m.stack) > maxStackLevel {
		return macFrame{}, &RuntimeError{
			Code:    "StackLevelTooDeep",
			Message: "maximum stack size exceeded",
			Loc:     m.ptr,
		}
	}

	// Set the stack values for the node.
	m.sSet(stackNodeIDPtr, reflect.ValueOf(n.Id))
	m.sSet(stackNodeDescPtr, reflect.ValueOf(n))

	// Increase the execution pointer
	m.ptr++

	switch n.Kind {
	case NodeIL_ROOT: // Root node executes all of it's children
		for _, c := range n.Children {
			_, err := c.call(ctx, m)
			if err != nil {
				return m.pop(), err
			}
		}
		return m.pop(), nil
	case NodeIL_VALUE: // Sets the value to the return pointer and returns.
		m.sSet(stackReturnPtr, n.Value.value())

		return m.pop(), nil
	case NodeIL_NAT:
		switch n.Value.Str {
		case "_delete":
			if len(n.Children) != 1 {
				return m.pop(), &RuntimeError{
					Code:    "NativeFunctionErr",
					Message: fmt.Sprintf("Func _delete expects 1 argument"),
				}
			}

			s, err := n.Children[0].call(ctx, m)
			if err != nil {
				return m.pop(), err
			}

			if r, ok := s[stackReturnPtr]; ok {
				if n, ok := r.Interface().(string); ok {
					delete(m.names, n)
				}
			}

			return m.pop(), nil
		default:
			return m.pop(), &RuntimeError{
				Code:    "UnknownNativeFunction",
				Message: fmt.Sprintf("no native function named %s", n.Value.Str),
			}
		}
	case NodeIL_FUNC: // Calls the function, and it's children
		// Make sure the function exists before doing more work.
		fn, err := m.lookup(n.Value.Str)
		if err != nil {
			return m.pop(), err
		}

		// Call for each child. The return values will be the arguments.
		args := []reflect.Value{}
		for _, c := range n.Children {
			s, err := c.call(ctx, m)
			if err != nil {
				return m.pop(), err
			}

			val, ret := s[stackReturnPtr]
			if !ret {
				return m.pop(), &RuntimeError{
					Code:    "MissingReturnValue",
					Message: fmt.Sprintf("no return value found for %s", c),
				}
			}

			args = append(args, val)
		}

		// Call the function passing in the arguments
		ret, err := fn.call(ctx, args)
		if err != nil {
			return m.pop(), err
		}

		// We actually return a value
		if fn.retC != 0 {
			m.sSet(stackReturnPtr, ret)
			if n.Chained != nil {
				s, err := n.Chained.call(context.WithValue(ctx, macCtxRetKey, ret), m)
				if err != nil {
					return m.pop(), err
				}

				// Override the return value from the chained call
				if r, ok := s[stackReturnPtr]; ok {
					m.sSet(stackReturnPtr, r)
				}
			}
		} else if n.Chained != nil { // We can't chain a function without a return value.
			return m.pop(), &RuntimeError{
				Code:    "ChainingToFunc",
				Message: fmt.Sprintf("Attempting to chain from '%s' but there is no return value", fn.name),
			}
		}

		return m.pop(), nil
	case NodeIL_GROUP:
		// The return value from the previous grouped function call.
		// Passed as the `LastReturn` to the next function call in the group
		var val reflect.Value

		// The list of grouped return values.
		grouped := []reflect.Value{}

		for _, c := range n.Children {
			st, err := c.call(context.WithValue(ctx, macCtxRetKey, val), m)
			if err != nil {
				return m.pop(), err
			}

			// If the child call stack has a return value, add it to the grouped return values.
			val, ret := st[stackReturnPtr]
			if ret {
				grouped = append(grouped, val)
			}
		}

		if n.Chained != nil {
			// Call the chained function passing in the slice of grouped return values as the `LastReturn`
			s, err := n.Chained.call(context.WithValue(ctx, macCtxRetKey, reflect.ValueOf(grouped)), m)
			if err != nil {
				return m.pop(), err
			}

			// If the chained function returns a value we need to set it as the group's last return value.
			// Groups do not return anything if the chain does not return anything.
			if r, ok := s[stackReturnPtr]; ok {
				m.sSet(stackReturnPtr, r)
			}
		}

		return m.pop(), nil
	case NodeIL_ASSIGN:
		name := n.Value.Str
		if name == "" { // Ensure we have a valid name.
			return m.pop(), &RuntimeError{
				Code:    "AssignmentError",
				Message: "Attempting to assing to a variable without a name.",
			}
		}
		if n.SubType != "const" { // Ensure we have a valid assignment type.
			return m.pop(), &RuntimeError{
				Code:    "AssignmentError",
				Message: "Attempting to assign to a non-constant",
			}
		}
		if n.Chained == nil { // Ensure we have something to get a value from.
			return m.pop(), &RuntimeError{
				Code:    "AssignmentError",
				Message: "Attempting to assign to without something to get the value from.",
			}
		}

		if _, ok := m.names[name]; ok {
			return m.pop(), &RuntimeError{
				Code:    "AssignmentError",
				Message: "Attempting to reassign a value to a constant.",
			}
		}

		// Get the value from the RHS of the assignment.
		s, err := n.Chained.call(ctx, m)
		if err != nil {
			return m.pop(), err
		}

		ret, ok := s[stackReturnPtr]
		if !ok {
			return m.pop(), &RuntimeError{
				Code:    "AssignmentError",
				Message: "Attempting to assign to but assignment RHS expression did not return a value.",
			}
		}

		// Store the variable name in the names
		m.names[name] = m.ptr
		// Store the variable value in the heap
		m.heap[m.ptr] = ret

		return m.pop(), nil
	case NodeIL_VAR:
		name := n.Value.Str
		if name == "" {
			return m.pop(), &RuntimeError{
				Code:    "VarErr",
				Message: "Attempting to fetch a named variable without a name",
			}
		}

		ptr, ok := m.names[name]
		if !ok {
			return m.pop(), &RuntimeError{
				Code:    "VarErr",
				Message: fmt.Sprintf("no variable named '%s'", name),
			}
		}

		val, ok := m.heap[ptr]
		if !ok {
			return m.pop(), &RuntimeError{
				Code:    "VarErr",
				Message: fmt.Sprintf("no variable named '%s'", name),
			}
		}

		m.sSet(stackReturnPtr, val)

		return m.pop(), nil
	default:
		return m.pop(), &RuntimeError{
			Code:    "UnknownInstruction",
			Message: fmt.Sprintf("Machine is not capable of executing %s", n.Kind.String()),
			Loc:     m.ptr,
		}
	}
}

// State returns the last state of the machine.
func (m *Machine) State() MacC {
	m.mu.RLock()
	s := m.lastState
	m.mu.RUnlock()

	return s
}

// RuntimeError represents and error raised when the machine encounters something it doesn't expect.
type RuntimeError struct {
	Code    string
	Message string
	Loc     uintptr
}

func (e RuntimeError) Error() string {
	return fmt.Sprintf("Runtime Error: <%s> %s", e.Code, e.Message)
}

type macCKeyType uint32

var (
	macCtxCurKey = macCKeyType(0)
	macCtxRetKey = macCKeyType(1)
)

// Mac returns the MacC from the context
func Mac(ctx context.Context) MacC {
	v := ctx.Value(macCtxCurKey)
	if t, ok := v.(MacC); ok {
		return t
	}
	return nil
}

// LastReturn gets the last returned value from the context
func LastReturn(ctx context.Context) interface{} {
	return ctx.Value(macCtxRetKey)
}

// Run takes raw source and compiles it and runs in a new machine.
func Run(i *Implementation, src string) error {
	p, err := CompileSource(src)
	if err != nil {
		return err
	}

	m := New(i)
	defer m.Shutdown()

	return m.Execute(p)
}
