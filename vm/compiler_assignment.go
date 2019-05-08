// Copyright (c) 2019 Open2b Software Snc. All rights reserved.
// https://www.open2b.com

// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vm

import (
	"reflect"

	"scrigo/ast"
)

// AddressType is the type of an element on the left side of and assignment.
type AddressType int8

const (
	AddressRegister           AddressType = iota // Variable assignments.
	AddressesBlank                               // Blank identifier assignments.
	AddressPackageVariable                       // Package variable assignments.
	AddressPointerIndirection                    // Pointer indirection assignments.
	AddressSliceIndex                            // Slice and array index assignments.
	AddressMapIndex                              // Map index assignments.
	AddressStructSelector                        // Struct selector assignments.
)

// Address represents an element on the left side of assignments.
type Address struct {
	c           *Compiler
	Type        AddressType
	ReflectType reflect.Type // Type of the addressed element.
	Reg1        int8         // Register containing the main expression.
	Reg2        int8         // Auxiliary register used in slice, map, array and selector assignments.
}

// NewAddress returns a new address. Meaning of reg1 and reg2 depends on address type.
func (c *Compiler) NewAddress(addrType AddressType, reflectType reflect.Type, reg1, reg2 int8) Address {
	return Address{c: c, Type: addrType, ReflectType: reflectType, Reg1: reg1, Reg2: reg2}
}

// Assign assigns value to a.
func (a Address) Assign(k bool, value int8, valueKind reflect.Kind) {
	switch a.Type {
	case AddressesBlank:
		// Nothing to do.
	case AddressRegister:
		a.c.fb.Move(k, value, a.Reg1, a.ReflectType.Kind(), valueKind)
	case AddressPointerIndirection:
		panic("TODO(Gianluca): not implemented")
	case AddressSliceIndex:
		a.c.fb.SetSlice(k, a.Reg1, value, a.Reg2, a.ReflectType.Elem().Kind())
	case AddressStructSelector:
		panic("TODO(Gianluca): not implemented")
	case AddressPackageVariable:
		if k {
			tmpReg := a.c.fb.NewRegister(valueKind)
			a.c.fb.Move(true, value, tmpReg, valueKind, valueKind)
			a.c.fb.SetVar(tmpReg, uint8(a.Reg1))
		} else {
			a.c.fb.SetVar(value, uint8(a.Reg1))
		}
	}
}

// assign assigns values to addresses.
func (c *Compiler) assign(addresses []Address, values []ast.Expression) {
	// TODO(Gianluca): use mayHaveDependencies.
	if len(addresses) == len(values) {
		valueRegs := make([]int8, len(values))
		valueTypes := make([]reflect.Type, len(values))
		valueIsK := make([]bool, len(values))
		for i := range values {
			valueTypes[i] = c.typeinfo[values[i]].Type
			valueRegs[i], valueIsK[i], _ = c.quickCompileExpr(values[i], valueTypes[i])
			if !valueIsK[i] {
				valueRegs[i] = c.fb.NewRegister(valueTypes[i].Kind())
				c.compileExpr(values[i], valueRegs[i], valueTypes[i])
			}
		}
		for i, addr := range addresses {
			addr.Assign(valueIsK[i], valueRegs[i], valueTypes[i].Kind())
		}
	} else {
		switch value := values[0].(type) {
		case *ast.Call:
			retRegs, retTypes := c.compileCall(value)
			for i, addr := range addresses {
				addr.Assign(false, retRegs[i], retTypes[i].Kind())
			}
		}
	}
}

// compileAssignmentNode compiles an assignment node.
func (c *Compiler) compileAssignmentNode(node *ast.Assignment) {
	switch node.Type {
	case ast.AssignmentDeclaration:
		addresses := make([]Address, len(node.Variables))
		for i, v := range node.Variables {
			v := v.(*ast.Identifier)
			varType := c.typeinfo[v].Type
			varReg := c.fb.NewRegister(varType.Kind())
			c.fb.BindVarReg(v.Name, varReg)
			addresses[i] = c.NewAddress(AddressRegister, varType, varReg, 0)
		}
		c.assign(addresses, node.Values)
	case ast.AssignmentSimple:
		addresses := make([]Address, len(node.Variables))
		for i, v := range node.Variables {
			switch v := v.(type) {
			case *ast.Identifier:
				if !isBlankIdentifier(v) {
					varType := c.typeinfo[v].Type
					reg := c.fb.ScopeLookup(v.Name)
					addresses[i] = c.NewAddress(AddressRegister, varType, reg, 0)
				} else {
					addresses[i] = c.NewAddress(AddressesBlank, reflect.Type(nil), 0, 0)
				}
			case *ast.Index:
				exprType := c.typeinfo[v.Expr].Type
				expr, _, isRegister := c.quickCompileExpr(v.Expr, exprType)
				if !isRegister {
					expr = c.fb.NewRegister(exprType.Kind())
					c.compileExpr(v.Expr, expr, exprType)
				}
				indexType := c.typeinfo[v.Index].Type
				index, _, isRegister := c.quickCompileExpr(v.Index, indexType)
				if !isRegister {
					index = c.fb.NewRegister(indexType.Kind())
					c.compileExpr(v.Index, index, indexType)
				}
				addrType := AddressSliceIndex
				if exprType.Kind() == reflect.Map {
					addrType = AddressMapIndex
				}
				addresses[i] = c.NewAddress(addrType, exprType, expr, index)
			case *ast.Selector:
				if variable, ok := c.availableVariables[v.Expr.(*ast.Identifier).Name+"."+v.Ident]; ok {
					varIndex := c.variableIndex(variable)
					addresses[i] = c.NewAddress(AddressPackageVariable, c.typeinfo[v].Type, int8(varIndex), 0)
				} else {
					panic("TODO(Gianluca): not implemented")
				}
			case *ast.UnaryOperator:
				if v.Operator() != ast.OperatorMultiplication {
					panic("bug: v.Operator() != ast.OperatorMultiplication") // TODO(Gianluca): remove.
				}
				panic("TODO(Gianluca): not implemented")
			default:
				panic("TODO(Gianluca): not implemented")
			}
		}
		c.assign(addresses, node.Values)
	default:
		var address Address
		var valueReg int8
		var valueType reflect.Type
		switch v := node.Variables[0].(type) {
		case *ast.Identifier:
			varType := c.typeinfo[v].Type
			reg := c.fb.ScopeLookup(v.Name)
			address = c.NewAddress(AddressRegister, varType, reg, 0)
			valueReg = reg
			valueType = varType
		case *ast.Index:
			exprType := c.typeinfo[v.Expr].Type
			expr, _, isRegister := c.quickCompileExpr(v.Expr, exprType)
			if !isRegister {
				expr = c.fb.NewRegister(exprType.Kind())
				c.compileExpr(v.Expr, expr, exprType)
			}
			indexType := c.typeinfo[v.Index].Type
			index, _, isRegister := c.quickCompileExpr(v.Index, indexType)
			if !isRegister {
				index = c.fb.NewRegister(indexType.Kind())
				c.compileExpr(v.Index, index, indexType)
			}
			addrType := AddressSliceIndex
			if exprType.Kind() == reflect.Map {
				addrType = AddressMapIndex
			}
			address = c.NewAddress(addrType, exprType, expr, index)
			valueType = exprType.Elem()
			valueReg = c.fb.NewRegister(valueType.Kind())
			c.fb.Index(false, expr, index, valueReg, exprType)
		default:
			panic("TODO(Gianluca): not implemented")
		}
		switch node.Type {
		case ast.AssignmentIncrement:
			c.fb.Add(true, valueReg, 1, valueReg, valueType.Kind())
		case ast.AssignmentDecrement:
			c.fb.Sub(true, valueReg, 1, valueReg, valueType.Kind())
		default:
			rightOpType := c.typeinfo[node.Values[0]].Type
			rightOp := c.fb.NewRegister(rightOpType.Kind())
			c.compileExpr(node.Values[0], rightOp, rightOpType)
			switch node.Type {
			case ast.AssignmentAddition:
				c.fb.Add(false, valueReg, rightOp, valueReg, valueType.Kind())
			case ast.AssignmentSubtraction:
				c.fb.Sub(false, valueReg, rightOp, valueReg, valueType.Kind())
			case ast.AssignmentMultiplication:
				c.fb.Mul(false, valueReg, rightOp, valueReg, valueType.Kind())
			case ast.AssignmentDivision:
				c.fb.Div(false, valueReg, rightOp, valueReg, valueType.Kind())
			case ast.AssignmentModulo:
				c.fb.Rem(false, valueReg, rightOp, valueReg, valueType.Kind())
			case ast.AssignmentLeftShift:
				panic("TODO(Gianluca): not implemented")
			case ast.AssignmentRightShift:
				panic("TODO(Gianluca): not implemented")
			case ast.AssignmentAnd:
				panic("TODO(Gianluca): not implemented")
			case ast.AssignmentOr:
				panic("TODO(Gianluca): not implemented")
			case ast.AssignmentXor:
				panic("TODO(Gianluca): not implemented")
			case ast.AssignmentAndNot:
				panic("TODO(Gianluca): not implemented")
			}
		}
		address.Assign(false, valueReg, valueType.Kind())
	}
}
