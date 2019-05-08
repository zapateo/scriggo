// Copyright (c) 2019 Open2b Software Snc. All rights reserved.
// https://www.open2b.com

// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vm

import (
	"fmt"
	"reflect"
	"scrigo/ast"
)

type FunctionBuilder struct {
	fn          *ScrigoFunction
	labels      []uint32
	gotos       map[uint32]uint32
	maxRegs     map[reflect.Kind]uint8 // max number of registers allocated at the same time.
	numRegs     map[reflect.Kind]uint8
	scopes      []map[string]int8
	scopeShifts []StackShift
}

// NewBuilder returns a new function builder for the function fn.
func NewBuilder(fn *ScrigoFunction) *FunctionBuilder {
	fn.Body = nil
	return &FunctionBuilder{
		fn:      fn,
		gotos:   map[uint32]uint32{},
		maxRegs: map[reflect.Kind]uint8{},
		numRegs: map[reflect.Kind]uint8{},
		scopes:  []map[string]int8{},
	}
}

// EnterScope enters a new scope.
// Every EnterScope call must be paired with a corresponding ExitScope call.
func (builder *FunctionBuilder) EnterScope() {
	builder.scopes = append(builder.scopes, map[string]int8{})
	builder.EnterStack()
}

// ExitScope exits last scope.
// Every ExitScope call must be paired with a corresponding EnterScope call.
func (builder *FunctionBuilder) ExitScope() {
	builder.scopes = builder.scopes[:len(builder.scopes)-1]
	builder.ExitStack()
}

// EnterStack enters a new virtual stack, whose registers will be reused (if
// necessary) after calling ExitScope.
// Every EnterStack call must be paired with a corresponding ExitStack call.
func (builder *FunctionBuilder) EnterStack() {
	scopeShift := StackShift{
		int8(builder.numRegs[reflect.Int]),
		int8(builder.numRegs[reflect.Float64]),
		int8(builder.numRegs[reflect.String]),
		int8(builder.numRegs[reflect.Interface]),
	}
	builder.scopeShifts = append(builder.scopeShifts, scopeShift)
}

// ExitStack exits current virtual stack, allowing its registers to be reused
// (if necessary).
// Every ExitStack call must be paired with a corresponding EnterStack call.
func (builder *FunctionBuilder) ExitStack() {
	shift := builder.scopeShifts[len(builder.scopeShifts)-1]
	builder.numRegs[reflect.Int] = uint8(shift[0])
	builder.numRegs[reflect.Float64] = uint8(shift[1])
	builder.numRegs[reflect.String] = uint8(shift[2])
	builder.numRegs[reflect.Interface] = uint8(shift[3])
	builder.scopeShifts = builder.scopeShifts[:len(builder.scopeShifts)-1]
}

// NewRegister makes a new register of a given kind.
func (builder *FunctionBuilder) NewRegister(kind reflect.Kind) int8 {
	switch kindToType(kind) {
	case TypeInt:
		kind = reflect.Int
	case TypeFloat:
		kind = reflect.Float64
	case TypeString:
		kind = reflect.String
	case TypeIface:
		kind = reflect.Interface
	}
	reg := int8(builder.numRegs[kind]) + 1
	builder.allocRegister(kind, reg)
	return reg
}

// BindVarReg binds name with register reg. To create a new variable, use
// VariableRegister in conjunction with BindVarReg.
func (builder *FunctionBuilder) BindVarReg(name string, reg int8) {
	builder.scopes[len(builder.scopes)-1][name] = reg
}

// IsVariable indicates if n is a variable (i.e. is a name defined in some of
// the current scopes).
func (builder *FunctionBuilder) IsVariable(n string) bool {
	for i := len(builder.scopes) - 1; i >= 0; i-- {
		_, ok := builder.scopes[i][n]
		if ok {
			return true
		}
	}
	return false
}

// ScopeLookup returns n's register.
func (builder *FunctionBuilder) ScopeLookup(n string) int8 {
	for i := len(builder.scopes) - 1; i >= 0; i-- {
		reg, ok := builder.scopes[i][n]
		if ok {
			return reg
		}
	}
	panic(fmt.Sprintf("bug: %s not found", n))
}

func (builder *FunctionBuilder) AddLine(pc uint32, line int) {
	if builder.fn.Lines == nil {
		builder.fn.Lines = map[uint32]int{pc: line}
	} else {
		builder.fn.Lines[pc] = line
	}
}

func (builder *FunctionBuilder) SetClosureRefs(refs []int16) {
	builder.fn.CRefs = refs
}

// SetFileLine sets the file name and line number of the Scrigo function.
func (builder *FunctionBuilder) SetFileLine(file string, line int) {
	builder.fn.File = file
	builder.fn.Line = line
}

// NewVariable returns a new variable.
func NewVariable(pkg, name string, value interface{}) Variable {
	return Variable{pkg, name, value}
}

// NewScrigoFunction returns a new Scrigo function with a given package, name
// and type.
func NewScrigoFunction(pkg, name string, typ reflect.Type) *ScrigoFunction {
	return &ScrigoFunction{Pkg: pkg, Name: name, Type: typ}
}

// NewNativeFunction returns a new native function with a given package, name
// and implementation. fn must be a function type.
func NewNativeFunction(pkg, name string, fn interface{}) *NativeFunction {
	return &NativeFunction{Pkg: pkg, Name: name, Fast: fn}
}

// AddType adds a type to the Scrigo function.
func (builder *FunctionBuilder) AddType(typ reflect.Type) uint8 {
	fn := builder.fn
	index := len(fn.Types)
	if index > 255 {
		panic("types limit reached")
	}
	for i, t := range fn.Types {
		if t == typ {
			return uint8(i)
		}
	}
	fn.Types = append(fn.Types, typ)
	return uint8(index)
}

// AddVariable adds a variable to the Scrigo function.
func (builder *FunctionBuilder) AddVariable(v Variable) uint8 {
	fn := builder.fn
	r := len(fn.Variables)
	if r > 255 {
		panic("variables limit reached")
	}
	fn.Variables = append(fn.Variables, v)
	return uint8(r)
}

// AddNativeFunction adds a native function to the Scrigo function.
func (builder *FunctionBuilder) AddNativeFunction(f *NativeFunction) uint8 {
	fn := builder.fn
	r := len(fn.NativeFunctions)
	if r > 255 {
		panic("native functions limit reached")
	}
	fn.NativeFunctions = append(fn.NativeFunctions, f)
	return uint8(r)
}

// AddScrigoFunction adds a Scrigo function to the Scrigo function.
func (builder *FunctionBuilder) AddScrigoFunction(f *ScrigoFunction) uint8 {
	fn := builder.fn
	r := len(fn.ScrigoFunctions)
	if r > 255 {
		panic("Scrigo functions limit reached")
	}
	fn.ScrigoFunctions = append(fn.ScrigoFunctions, f)
	return uint8(r)
}

// MakeStringConstant makes a new string constant, returning it's index.
func (builder *FunctionBuilder) MakeStringConstant(c string) int8 {
	r := len(builder.fn.Constants.String)
	if r > 255 {
		panic("string refs limit reached")
	}
	builder.fn.Constants.String = append(builder.fn.Constants.String, c)
	return int8(r)
}

// MakeGeneralConstant makes a new general constant, returning it's index.
func (builder *FunctionBuilder) MakeGeneralConstant(v interface{}) int8 {
	r := len(builder.fn.Constants.General)
	if r > 255 {
		panic("general refs limit reached")
	}
	builder.fn.Constants.General = append(builder.fn.Constants.General, v)
	return int8(r)
}

// MakeFloatConstant makes a new float constant, returning it's index.
func (builder *FunctionBuilder) MakeFloatConstant(c float64) int8 {
	r := len(builder.fn.Constants.Float)
	if r > 255 {
		panic("float refs limit reached")
	}
	builder.fn.Constants.Float = append(builder.fn.Constants.Float, c)
	return int8(r)
}

// MakeIntConstant makes a new int constant, returning it's index.
func (builder *FunctionBuilder) MakeIntConstant(c int64) int8 {
	r := len(builder.fn.Constants.Int)
	if r > 255 {
		panic("int refs limit reached")
	}
	builder.fn.Constants.Int = append(builder.fn.Constants.Int, c)
	return int8(r)
}

func (builder *FunctionBuilder) MakeInterfaceConstant(c interface{}) int8 {
	r := -len(builder.fn.Constants.General) - 1
	if r == -129 {
		panic("interface refs limit reached")
	}
	builder.fn.Constants.General = append(builder.fn.Constants.General, c)
	return int8(r)
}

// CurrentAddr returns builder's current address.
func (builder *FunctionBuilder) CurrentAddr() uint32 {
	return uint32(len(builder.fn.Body))
}

// NewLabel creates a new empty label. Use SetLabelAddr to associate an address
// to it.
func (builder *FunctionBuilder) NewLabel() uint32 {
	builder.labels = append(builder.labels, uint32(0))
	return uint32(len(builder.labels))
}

// SetLabelAddr sets label's address as builder's current address.
func (builder *FunctionBuilder) SetLabelAddr(label uint32) {
	builder.labels[label-1] = builder.CurrentAddr()
}

var intType = reflect.TypeOf(0)
var float64Type = reflect.TypeOf(0.0)
var stringType = reflect.TypeOf("")
var emptyInterfaceType = reflect.TypeOf(&[]interface{}{interface{}(nil)}[0]).Elem()

func encodeAddr(v uint32) (a, b, c int8) {
	a = int8(uint8(v))
	b = int8(uint8(v >> 8))
	c = int8(uint8(v >> 16))
	return
}

// Type returns typ's index, creating it if necessary.
func (builder *FunctionBuilder) Type(typ reflect.Type) int8 {
	var tr int8
	var found bool
	types := builder.fn.Types
	for i, t := range types {
		if t == typ {
			tr = int8(i)
			found = true
		}
	}
	if !found {
		if len(types) == 256 {
			panic("types limit reached")
		}
		tr = int8(len(types))
		builder.fn.Types = append(types, typ)
	}
	return tr
}

func (builder *FunctionBuilder) End() {
	fn := builder.fn
	for addr, label := range builder.gotos {
		i := fn.Body[addr]
		i.A, i.B, i.C = encodeAddr(builder.labels[label-1])
		fn.Body[addr] = i
	}
	builder.gotos = nil
	for kind, num := range builder.maxRegs {
		switch {
		case reflect.Int <= kind && kind <= reflect.Uint64:
			if num > fn.RegNum[0] {
				fn.RegNum[0] = num
			}
		case kind == reflect.Float64 || kind == reflect.Float32:
			if num > fn.RegNum[1] {
				fn.RegNum[1] = num
			}
		case kind == reflect.String:
			if num > fn.RegNum[2] {
				fn.RegNum[2] = num
			}
		default:
			if num > fn.RegNum[3] {
				fn.RegNum[3] = num
			}
		}
	}

}

func (builder *FunctionBuilder) allocRegister(kind reflect.Kind, reg int8) {
	switch kindToType(kind) {
	case TypeInt:
		kind = reflect.Int
	case TypeFloat:
		kind = reflect.Float64
	case TypeString:
		kind = reflect.String
	case TypeIface:
		kind = reflect.Interface
	}
	if reg > 0 {
		if num, ok := builder.maxRegs[kind]; !ok || uint8(reg) > num {
			builder.maxRegs[kind] = uint8(reg)
		}
		if num, ok := builder.numRegs[kind]; !ok || uint8(reg) > num {
			builder.numRegs[kind] = uint8(reg)
		}
	}
}

// Add appends a new "add" instruction to the function body.
//
//     z = x + y
//
func (builder *FunctionBuilder) Add(k bool, x, y, z int8, kind reflect.Kind) {
	var op Operation
	builder.allocRegister(kind, x)
	if !k {
		builder.allocRegister(kind, y)
	}
	builder.allocRegister(kind, z)
	switch kind {
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
		op = OpAddInt
	case reflect.Int32, reflect.Uint32:
		op = OpAddInt32
	case reflect.Int16, reflect.Uint16:
		op = OpAddInt16
	case reflect.Int8, reflect.Uint8:
		op = OpAddInt8
	case reflect.Float64:
		op = OpAddFloat64
	case reflect.Float32:
		op = OpAddFloat32
	default:
		panic("add: invalid type")
	}
	if k {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: y, C: z})
}

// Append appends a new "Append" instruction to the function body.
//
//     s = append(s, regs[first:first+length]...)
//
func (builder *FunctionBuilder) Append(first, length, s int8) {
	builder.allocRegister(reflect.Interface, s)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpAppend, A: first, B: length, C: s})
}

// AppendSlice appends a new "AppendSlice" instruction to the function body.
//
//     s = append(s, t)
//
func (builder *FunctionBuilder) AppendSlice(t, s int8) {
	builder.allocRegister(reflect.Interface, t)
	builder.allocRegister(reflect.Interface, s)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpAppendSlice, A: t, C: s})
}

// Assert appends a new "assert" instruction to the function body.
//
//     z = e.(t)
//
func (builder *FunctionBuilder) Assert(e int8, typ reflect.Type, z int8) {
	var op Operation
	var tr int8
	builder.allocRegister(reflect.Interface, e)
	switch typ {
	case intType:
		builder.allocRegister(reflect.Int, z)
		op = OpAssertInt
	case float64Type:
		builder.allocRegister(reflect.Float64, z)
		op = OpAssertFloat64
	case stringType:
		builder.allocRegister(reflect.String, z)
		op = OpAssertString
	default:
		builder.allocRegister(reflect.Interface, z)
		op = OpAssert
		var found bool
		for i, t := range builder.fn.Types {
			if t == typ {
				tr = int8(i)
				found = true
			}
		}
		if !found {
			tr = int8(len(builder.fn.Types))
			builder.fn.Types = append(builder.fn.Types, typ)
		}
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: e, B: tr, C: z})
}

// BinaryBitOperation appends a new binary bit operation specified by operator
// to the function body.
//
//	dst = x op y
//
func (builder *FunctionBuilder) BinaryBitOperation(operator ast.OperatorType, ky bool, x, y, dst int8, kind reflect.Kind) {
	// TODO(Gianluca): should builder be dependent from ast? If no, introduce
	// a new type which describes the operator.
	var op Operation
	switch operator {
	case ast.OperatorAnd:
		op = OpAnd
	case ast.OperatorOr:
		op = OpOr
	case ast.OperatorXor:
		op = OpXor
	case ast.OperatorAndNot:
		op = OpAndNot
	case ast.OperatorLeftShift:
		op = OpLeftShift
		switch kind {
		case reflect.Int8, reflect.Uint8:
			op = OpLeftShift8
		case reflect.Int16, reflect.Uint16:
			op = OpLeftShift16
		case reflect.Int32, reflect.Uint32:
			op = OpLeftShift32
		}
	case ast.OperatorRightShift:
		op = OpRightShift
		switch kind {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			op = OpRightShiftU
		}
	}
	if ky {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: y, C: dst})
}

// Bind appends a new "Bind" instruction to the function body.
//
//     r = cv
//
func (builder *FunctionBuilder) Bind(cv uint8, r int8) {
	builder.allocRegister(reflect.Interface, r)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpBind, B: int8(cv), C: r})
}

// Call appends a new "Call" instruction to the function body.
//
//     p.f()
//
func (builder *FunctionBuilder) Call(f int8, shift StackShift, line int) {
	var fn = builder.fn
	fn.Body = append(fn.Body, Instruction{Op: OpCall, A: f})
	fn.Body = append(fn.Body, Instruction{Op: Operation(shift[0]), A: shift[1], B: shift[2], C: shift[3]})
	builder.AddLine(uint32(len(fn.Body)-2), line)
}

// CallNative appends a new "CallNative" instruction to the function body.
//
//     p.F()
//
func (builder *FunctionBuilder) CallNative(f int8, numVariadic int8, shift StackShift) {
	var fn = builder.fn
	fn.Body = append(fn.Body, Instruction{Op: OpCallNative, A: f, C: numVariadic})
	fn.Body = append(fn.Body, Instruction{Op: Operation(shift[0]), A: shift[1], B: shift[2], C: shift[3]})
}

// CallIndirect appends a new "CallIndirect" instruction to the function body.
//
//     f()
//
func (builder *FunctionBuilder) CallIndirect(f int8, numVariadic int8, shift StackShift) {
	var fn = builder.fn
	fn.Body = append(fn.Body, Instruction{Op: OpCallIndirect, A: f, C: numVariadic})
	fn.Body = append(fn.Body, Instruction{Op: Operation(shift[0]), A: shift[1], B: shift[2], C: shift[3]})
}

// Assert appends a new "cap" instruction to the function body.
//
//     z = cap(s)
//
func (builder *FunctionBuilder) Cap(s, z int8) {
	builder.allocRegister(reflect.Interface, s)
	builder.allocRegister(reflect.Int, z)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpCap, A: s, C: z})
}

// Case appends a new "Case" instruction to the function body.
//
//     case ch <- value
//     case value = <-ch
//     default
//
func (builder *FunctionBuilder) Case(kvalue bool, dir reflect.SelectDir, value, ch int8, kind reflect.Kind) {
	if !kvalue && value != 0 {
		builder.allocRegister(kind, value)
	}
	if ch != 0 {
		builder.allocRegister(reflect.Interface, ch)
	}
	op := OpCase
	if kvalue {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: int8(dir), B: value, C: ch})
}

// Concat appends a new "concat" instruction to the function body.
//
//     z = concat(s, t)
//
func (builder *FunctionBuilder) Concat(s, t, z int8) {
	builder.allocRegister(reflect.Interface, s)
	builder.allocRegister(reflect.Interface, t)
	builder.allocRegister(reflect.Interface, z)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpConcat, A: s, B: t, C: z})
}

// Convert appends a new "Convert" instruction to the function body.
//
// 	 dst = typ(src)
//
func (builder *FunctionBuilder) Convert(src int8, typ reflect.Type, dst int8, srcKind reflect.Kind) {
	regType := builder.Type(typ)
	builder.allocRegister(reflect.Interface, dst)
	var op Operation
	switch kindToType(srcKind) {
	case TypeIface:
		op = OpConvert
	case TypeInt:
		switch srcKind {
		case reflect.Uint,
			reflect.Uint8,
			reflect.Uint16,
			reflect.Uint32,
			reflect.Uint64,
			reflect.Uintptr:
			op = OpConvertUint
		default:
			op = OpConvertInt
		}
	case TypeString:
		op = OpConvertString
	case TypeFloat:
		op = OpConvertFloat
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: src, B: regType, C: dst})
}

// Copy appends a new "Copy" instruction to the function body.
//
//     n == 0:   copy(dst, src)
// 	 n != 0:   n := copy(dst, src)
//
func (builder *FunctionBuilder) Copy(dst, src, n int8) {
	builder.allocRegister(reflect.Interface, dst)
	builder.allocRegister(reflect.Interface, src)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpCopy, A: src, B: n, C: dst})
}

// Defer appends a new "Defer" instruction to the function body.
//
//     defer
//
func (builder *FunctionBuilder) Defer(f int8, numVariadic int8, off, arg StackShift) {
	var fn = builder.fn
	builder.allocRegister(reflect.Interface, f)
	fn.Body = append(fn.Body, Instruction{Op: OpDefer, A: f, C: numVariadic})
	fn.Body = append(fn.Body, Instruction{Op: Operation(off[0]), A: off[1], B: off[2], C: off[3]})
	fn.Body = append(fn.Body, Instruction{Op: Operation(arg[0]), A: arg[1], B: arg[2], C: arg[3]})
}

// Delete appends a new "delete" instruction to the function body.
//
//     delete(m, k)
//
func (builder *FunctionBuilder) Delete(m, k int8) {
	builder.allocRegister(reflect.Interface, m)
	builder.allocRegister(reflect.Interface, k)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpDelete, A: m, B: k})
}

// Div appends a new "div" instruction to the function body.
//
//     z = x / y
//
func (builder *FunctionBuilder) Div(ky bool, x, y, z int8, kind reflect.Kind) {
	builder.allocRegister(kind, x)
	builder.allocRegister(kind, y)
	builder.allocRegister(kind, z)
	var op Operation
	switch kind {
	case reflect.Int, reflect.Int64:
		op = OpDivInt
	case reflect.Int32:
		op = OpDivInt32
	case reflect.Int16:
		op = OpDivInt16
	case reflect.Int8:
		op = OpDivInt8
	case reflect.Uint, reflect.Uint64:
		op = OpDivUint64
	case reflect.Uint32:
		op = OpDivUint32
	case reflect.Uint16:
		op = OpDivUint16
	case reflect.Uint8:
		op = OpDivUint8
	case reflect.Float64:
		op = OpDivFloat64
	case reflect.Float32:
		op = OpDivFloat32
	default:
		panic("div: invalid type")
	}
	if ky {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: y, C: z})
}

// Range appends a new "Range" instruction to the function body.
//
//	TODO
//
func (builder *FunctionBuilder) Range(expr int8, kind reflect.Kind) {
	switch kind {
	case reflect.String:
		builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpRangeString, C: expr})
	default:
		panic("TODO: not implemented")
	}
}

// Func appends a new "Func" instruction to the function body.
//
//     r = func() { ... }
//
func (builder *FunctionBuilder) Func(r int8, typ reflect.Type) *ScrigoFunction {
	b := len(builder.fn.Literals)
	if b == 256 {
		panic("ScrigoFunctions limit reached")
	}
	builder.allocRegister(reflect.Interface, r)
	fn := &ScrigoFunction{
		Type:   typ,
		Parent: builder.fn,
	}
	builder.fn.Literals = append(builder.fn.Literals, fn)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpFunc, B: int8(b), C: r})
	return fn
}

// GetFunc appends a new "GetFunc" instruction to the function body.
//
//     z = p.f
//
func (builder *FunctionBuilder) GetFunc(native bool, f int8, z int8) {
	builder.allocRegister(reflect.Interface, z)
	var a int8
	if native {
		a = 1
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpGetFunc, A: a, B: f, C: z})
}

// GetVar appends a new "GetVar" instruction to the function body.
//
//     z = p.v
//
func (builder *FunctionBuilder) GetVar(v uint8, z int8) {
	builder.allocRegister(reflect.Interface, z)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpGetVar, A: int8(v), C: z})
}

// Go appends a new "Go" instruction to the function body.
//
//     go
//
func (builder *FunctionBuilder) Go() {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpGo})
}

// Goto appends a new "goto" instruction to the function body.
//
//     goto label
//
func (builder *FunctionBuilder) Goto(label uint32) {
	in := Instruction{Op: OpGoto}
	if label > 0 {
		if label > uint32(len(builder.labels)) {
			panic("bug!") // TODO(Gianluca): remove.
		}
		addr := builder.labels[label-1]
		if addr == 0 {
			builder.gotos[builder.CurrentAddr()] = label
		} else {
			in.A, in.B, in.C = encodeAddr(addr)
		}
	}
	builder.fn.Body = append(builder.fn.Body, in)
}

// If appends a new "If" instruction to the function body.
//
//     x
//     !x
//     x == y
//     x != y
//     x <  y
//     x <= y
//     x >  y
//     x >= y
//     x == nil
//     x != nil
//     len(x) == y
//     len(x) != y
//     len(x) <  y
//     len(x) <= y
//     len(x) >  y
//     len(x) >= y
//
func (builder *FunctionBuilder) If(k bool, x int8, o Condition, y int8, kind reflect.Kind) {
	builder.allocRegister(kind, x)
	if !k {
		builder.allocRegister(kind, y)
	}
	var op Operation
	switch kindToType(kind) {
	case TypeInt:
		op = OpIfInt
	case TypeFloat:
		op = OpIfFloat
	case TypeString:
		op = OpIfString
	case TypeIface:
		panic("If: invalid type")
	}
	if k {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: int8(o), C: y})
}

// Index appends a new "index" instruction to the function body
//
//	dst = expr[i]
//
func (builder *FunctionBuilder) Index(ki bool, expr, i, dst int8, exprType reflect.Type) {
	kind := exprType.Kind()
	var op Operation
	switch kind {
	default:
		op = OpIndex
	case reflect.Slice:
		op = OpSliceIndex
	case reflect.String:
		op = OpStringIndex
	case reflect.Map:
		op = OpMapIndex
	}
	if ki {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: expr, B: i, C: dst})
}

// Len appends a new "len" instruction to the function body.
//
//     l = len(s)
//
func (builder *FunctionBuilder) Len(s, l int8, t reflect.Type) {
	builder.allocRegister(reflect.Interface, s)
	builder.allocRegister(reflect.Int, l)
	var a int8
	switch t {
	case reflect.TypeOf(""):
		// TODO(Gianluca): this case catches string types only, not defined
		// types with underlying type string.
		a = 0
	default:
		a = 1
	case reflect.TypeOf([]byte{}):
		a = 2
	case reflect.TypeOf([]string{}):
		a = 4
	case reflect.TypeOf([]interface{}{}):
		a = 5
	case reflect.TypeOf(map[string]string{}):
		a = 6
	case reflect.TypeOf(map[string]int{}):
		a = 7
	case reflect.TypeOf(map[string]interface{}{}):
		a = 8
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpLen, A: a, B: s, C: l})
}

// LoadNumber appends a new "LoadNumber" instruction to the function body.
//
func (builder *FunctionBuilder) LoadNumber(typ Type, index, dst int8) {
	var a int8
	switch typ {
	case TypeInt:
		a = 0
	case TypeFloat:
		a = 1
	default:
		panic("LoadNumber only accepts TypeInt or TypeFloat as type")
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpLoadNumber, A: a, B: index, C: dst})
}

// MakeChan appends a new "MakeChan" instruction to the function body.
//
//     dst = make(typ, capacity)
//
func (builder *FunctionBuilder) MakeChan(typ int8, kCapacity bool, capacity int8, dst int8) {
	// TODO(Gianluca): uniform all Make* functions to take a reflect.Type or a
	// type index (int8).
	op := OpMakeChan
	if kCapacity {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: typ, B: capacity, C: dst})
}

// MakeMap appends a new "MakeMap" instruction to the function body.
//
//     dst = make(typ, size)
//
func (builder *FunctionBuilder) MakeMap(typ int8, kSize bool, size int8, dst int8) {
	op := OpMakeMap
	if kSize {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: typ, B: size, C: dst})
}

// MakeSlice appends a new "MakeSlice" instruction to the function body.
//
//     make(sliceType, len, cap)
//
func (builder *FunctionBuilder) MakeSlice(kLen, kCap bool, sliceType reflect.Type, len, cap, dst int8) {
	builder.allocRegister(reflect.Interface, dst)
	t := builder.Type(sliceType)
	var k int8
	if len == 0 && cap == 0 {
		k = 1
	} else {
		if kLen {
			k |= 1 << 1
		}
		if kCap {
			k |= 1 << 2
		}
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpMakeSlice, A: t, B: k, C: dst})
	if k > 1 {
		builder.fn.Body = append(builder.fn.Body, Instruction{A: len, B: cap})
	}
}

// Move appends a new "Move" instruction to the function body.
//
//     z = x
//
func (builder *FunctionBuilder) Move(k bool, x, z int8, srcKind, dstKind reflect.Kind) {
	if !k {
		builder.allocRegister(srcKind, x)
	}
	builder.allocRegister(srcKind, z)
	op := OpMove
	if k {
		op = -op
	}
	var moveType MoveType
	switch kindToType(dstKind) {
	case TypeInt:
		moveType = IntInt
	case TypeFloat:
		moveType = FloatFloat
	case TypeString:
		moveType = StringString
	case TypeIface:
		switch kindToType(srcKind) {
		case TypeInt:
			moveType = IntGeneral
		case TypeFloat:
			moveType = FloatGeneral
		case TypeString:
			moveType = StringGeneral
		case TypeIface:
			moveType = GeneralGeneral
		}
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: int8(moveType), B: x, C: z})
}

// Mul appends a new "mul" instruction to the function body.
//
//     z = x * y
//
func (builder *FunctionBuilder) Mul(ky bool, x, y, z int8, kind reflect.Kind) {
	builder.allocRegister(kind, x)
	builder.allocRegister(kind, y)
	builder.allocRegister(kind, z)
	var op Operation
	switch kind {
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
		op = OpMulInt
	case reflect.Int32, reflect.Uint32:
		op = OpMulInt32
	case reflect.Int16, reflect.Uint16:
		op = OpMulInt16
	case reflect.Int8, reflect.Uint8:
		op = OpMulInt8
	case reflect.Float64:
		op = OpMulFloat64
	case reflect.Float32:
		op = OpMulFloat32
	default:
		panic("mul: invalid type")
	}
	if ky {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: y, C: z})
}

// New appends a new "new" instruction to the function body.
//
//     z = new(t)
//
func (builder *FunctionBuilder) New(typ reflect.Type, z int8) {
	builder.allocRegister(reflect.Interface, z)
	a := builder.AddType(typ)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpNew, A: int8(a), C: z})
}

// Nop appends a new "Nop" instruction to the function body.
//
func (builder *FunctionBuilder) Nop() {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpNone})
}

// Panic appends a new "Panic" instruction to the function body.
//
//     panic(v)
//
func (builder *FunctionBuilder) Panic(v int8, line int) {
	fn := builder.fn
	builder.allocRegister(reflect.Interface, v)
	fn.Body = append(fn.Body, Instruction{Op: OpPanic, A: v})
	builder.AddLine(uint32(len(fn.Body)-1), line)
}

// Print appends a new "Print" instruction to the function body.
//
//     print(arg)
//
func (builder *FunctionBuilder) Print(arg int8) {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpPrint, A: arg})
}

// Receive appends a new "Receive" instruction to the function body.
//
//	dst = <- ch
//
//	dst, ok = <- ch
//
func (builder *FunctionBuilder) Receive(ch, ok, dst int8) {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpReceive, A: ch, B: ok, C: dst})
}

// Recover appends a new "Recover" instruction to the function body.
//
//     recover()
//
func (builder *FunctionBuilder) Recover(r int8) {
	builder.allocRegister(reflect.Interface, r)
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpRecover, C: r})
}

// Rem appends a new "rem" instruction to the function body.
//
//     z = x % y
//
func (builder *FunctionBuilder) Rem(ky bool, x, y, z int8, kind reflect.Kind) {
	builder.allocRegister(kind, x)
	builder.allocRegister(kind, y)
	builder.allocRegister(kind, z)
	var op Operation
	switch kind {
	case reflect.Int, reflect.Int64:
		op = OpRemInt
	case reflect.Int32:
		op = OpRemInt32
	case reflect.Int16:
		op = OpRemInt16
	case reflect.Int8:
		op = OpRemInt8
	case reflect.Uint, reflect.Uint64:
		op = OpRemUint64
	case reflect.Uint32:
		op = OpRemUint32
	case reflect.Uint16:
		op = OpRemUint16
	case reflect.Uint8:
		op = OpRemUint8
	default:
		panic("rem: invalid type")
	}
	if ky {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: y, C: z})
}

// Return appends a new "return" instruction to the function body.
//
//     return
//
func (builder *FunctionBuilder) Return() {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpReturn})
}

// Select appends a new "Select" instruction to the function body.
//
//     select
//
func (builder *FunctionBuilder) Select() {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpSelect})
}

// Selector appends a new "Selector" instruction to the function body.
//
// 	C = A.field
//
func (builder *FunctionBuilder) Selector(a, field, c int8) {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpSelector, A: a, B: field, C: c})
}

// Send appends a new "Send" instruction to the function body.
//
//	ch <- v
//
func (builder *FunctionBuilder) Send(ch, v int8) {
	// TODO(Gianluca): how can send know kind/type?
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpSend, A: v, C: ch})
}

// SetVar appends a new "SetVar" instruction to the function body.
//
//     p.v = r
//
func (builder *FunctionBuilder) SetVar(r int8, v uint8) {
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: OpSetVar, B: r, C: int8(v)})
}

// SetMap appends a new "SetMap" instruction to the function body.
//
//	m[key] = value
//
func (builder *FunctionBuilder) SetMap(k bool, m, value, key int8) {
	op := OpSetMap
	if k {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: m, B: value, C: key})
}

// SetSlice appends a new "SetSlice" instruction to the function body.
//
//	slice[index] = value
//
func (builder *FunctionBuilder) SetSlice(k bool, slice, value, index int8, elemKind reflect.Kind) {
	_ = elemKind // TODO(Gianluca): remove.
	in := Instruction{Op: OpSetSlice, A: slice, B: value, C: index}
	if k {
		in.Op = -in.Op
	}
	builder.fn.Body = append(builder.fn.Body, in)
}

// Sub appends a new "Sub" instruction to the function body.
//
//     z = x - y
//
func (builder *FunctionBuilder) Sub(k bool, x, y, z int8, kind reflect.Kind) {
	builder.allocRegister(reflect.Int, x)
	if !k {
		builder.allocRegister(reflect.Int, y)
	}
	builder.allocRegister(reflect.Int, z)
	var op Operation
	switch kind {
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
		op = OpSubInt
	case reflect.Int32, reflect.Uint32:
		op = OpSubInt32
	case reflect.Int16, reflect.Uint16:
		op = OpSubInt16
	case reflect.Int8, reflect.Uint8:
		op = OpSubInt8
	case reflect.Float64:
		op = OpSubFloat64
	case reflect.Float32:
		op = OpSubFloat32
	default:
		panic("sub: invalid type")
	}
	if k {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: y, C: z})
}

// SubInv appends a new "SubInv" instruction to the function body.
//
//     z = y - x
//
func (builder *FunctionBuilder) SubInv(k bool, x, y, z int8, kind reflect.Kind) {
	builder.allocRegister(reflect.Int, x)
	if !k {
		builder.allocRegister(reflect.Int, y)
	}
	builder.allocRegister(reflect.Int, z)
	var op Operation
	switch kind {
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
		op = OpSubInvInt
	case reflect.Int32, reflect.Uint32:
		op = OpSubInvInt32
	case reflect.Int16, reflect.Uint16:
		op = OpSubInvInt16
	case reflect.Int8, reflect.Uint8:
		op = OpSubInvInt8
	case reflect.Float64:
		op = OpSubInvFloat64
	case reflect.Float32:
		op = OpSubInvFloat32
	default:
		panic("subInv: invalid type")
	}
	if k {
		op = -op
	}
	builder.fn.Body = append(builder.fn.Body, Instruction{Op: op, A: x, B: y, C: z})
}

// TailCall appends a new "TailCall" instruction to the function body.
//
//     f()
//
func (builder *FunctionBuilder) TailCall(f int8, line int) {
	var fn = builder.fn
	fn.Body = append(fn.Body, Instruction{Op: OpTailCall, A: f})
	builder.AddLine(uint32(len(fn.Body)-1), line)
}
