// Copyright (c) 2019 Open2b Software Snc. All rights reserved.
// https://www.open2b.com

// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"reflect"
)

// definedType represents a type defined in the Scriggo compiled code with a
// type definition, where the underlying type can be both a type compiled in
// the Scriggo code or in gc.
type definedType struct {
	// The embedded reflect.Type can be both a reflect.Type implemented by the
	// package "reflect" or a ScriggoType. In the other implementations of
	// ScriggoType the embedded reflect.Type is always a gc compiled type.
	reflect.Type

	name string
}

// DefinedOf returns the defined type with the given name and underlying type.
// For example, if n is "Int" and k represents int, DefinedOf(n, k) represents
// the type Int declared with 'type Int int'.
func (types *Types) DefinedOf(name string, underlyingType reflect.Type) reflect.Type {
	if name == "" {
		panic("BUG: name cannot be empty")
	}
	return definedType{Type: underlyingType, name: name}
}

func (x definedType) Name() string {
	return x.name
}

func (x definedType) AssignableTo(y reflect.Type) bool {
	return AssignableTo(x, y)
}

func (x definedType) ConvertibleTo(y reflect.Type) bool {
	return ConvertibleTo(x, y)
}

func (x definedType) Implements(y reflect.Type) bool {
	return Implements(x, y)
}

func (x definedType) MethodByName(string) (reflect.Method, bool) {
	// TODO.
	return reflect.Method{}, false
}

func (x definedType) String() string {
	// For defined types the string representation is exactly the name of the
	// type; the internal structure of the type is hidden.
	// TODO: verify that this is correct.
	return x.name
}

// Underlying implements the interface runtime.Wrapper.
func (x definedType) Underlying() reflect.Type {
	if st, ok := x.Type.(ScriggoType); ok {
		return st.Underlying()
	}
	assertNotScriggoType(x.Type)
	return x.Type
}

// Unwrap implements the interface runtime.Wrapper.
func (x definedType) Unwrap(v reflect.Value) (reflect.Value, bool) { return unwrap(x, v) }

// Wrap implements the interface runtime.Wrapper.
func (x definedType) Wrap(v reflect.Value) reflect.Value { return wrap(x, v) }