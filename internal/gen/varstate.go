// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gen

import (
	"github.com/tsavola/wag/internal/values"
)

type VarState struct {
	Cache       values.Operand
	RefCount    int
	Dirty       bool
	BoundsStack []values.Bounds
}

func (v *VarState) ResetCache() {
	v.Cache = values.NoOperand(v.Cache.Type)
	v.Dirty = false
}

func (v *VarState) TrimBoundsStack(size int) {
	if len(v.BoundsStack) > size {
		v.BoundsStack = v.BoundsStack[:size]
	}
}