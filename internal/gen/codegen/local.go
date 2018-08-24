// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codegen

import (
	"fmt"

	"github.com/tsavola/wag/internal/gen"
	"github.com/tsavola/wag/internal/loader"
	"github.com/tsavola/wag/internal/regs"
	"github.com/tsavola/wag/internal/values"
)

func genGetLocal(f *gen.Func, load loader.L, op Opcode, info opInfo) (deadend bool) {
	localIndex := load.Varuint32()
	if localIndex >= uint32(len(f.Vars)) {
		panic(fmt.Errorf("%s index out of bounds: %d", op, localIndex))
	}

	pushVarOperand(f, int32(localIndex))
	return
}

func genSetLocal(f *gen.Func, load loader.L, op Opcode, info opInfo) (deadend bool) {
	localIndex := load.Varuint32()
	if localIndex >= uint32(len(f.Vars)) {
		panic(fmt.Errorf("%s index out of bounds: %d", op, localIndex))
	}

	opSetLocal(f, op, int32(localIndex))
	return
}

func genTeeLocal(f *gen.Func, load loader.L, op Opcode, info opInfo) (deadend bool) {
	localIndex := load.Varuint32()
	if localIndex >= uint32(len(f.Vars)) {
		panic(fmt.Errorf("%s index out of bounds: %d", op, localIndex))
	}

	opSetLocal(f, op, int32(localIndex))
	pushVarOperand(f, int32(localIndex))
	return
}

func opSetLocal(f *gen.Func, op Opcode, index int32) {
	debugf("setting variable #%d", index)

	v := &f.Vars[index]
	t := v.Cache.Type

	newValue := popOperand(f)
	if newValue.Type != t {
		panic(fmt.Errorf("%s %s variable #%d with wrong operand type: %s", op, t, index, newValue.Type))
	}

	switch newValue.Storage {
	case values.Imm:
		if v.Cache.Storage == values.Imm && newValue.ImmValue() == v.Cache.ImmValue() {
			return // nop
		}

	case values.VarReference:
		if newValue.VarIndex() == index {
			return // nop
		}
	}

	debugf("variable reference count: %d", v.RefCount)

	if v.RefCount > 0 {
		// detach all references by copying to temp regs or spilling to stack

		switch v.Cache.Storage {
		case values.Nowhere, values.VarReg:
			var spillUntil int

			for i := len(f.Operands) - 1; i >= 0; i-- {
				x := f.Operands[i]

				if x.Storage == values.VarReference && x.VarIndex() == index {
					reg, ok := f.Regs.Alloc(t)
					if !ok {
						spillUntil = i
						goto spill
					}

					zeroExt := opMove(f, reg, x, true) // TODO: avoid multiple loads
					f.Operands[i] = values.TempRegOperand(t, reg, zeroExt)

					v.RefCount--
					if v.RefCount == 0 {
						goto done
					}
				}
			}

			panic("could not find all variable references")

		spill:
			opInitVars(f)

			for i := 0; i <= spillUntil; i++ {
				x := f.Operands[i]
				var done bool

				switch x.Storage {
				case values.VarReference:
					f.Vars[x.VarIndex()].RefCount--
					done = (x.VarIndex() == index && v.RefCount == 0)
					fallthrough
				case values.TempReg, values.ConditionFlags:
					opPush(f, x)
					f.Operands[i] = values.StackOperand(x.Type)
				}

				if done {
					goto done
				}
			}

			panic("could not find all variable references")

		done:
		}
	}

	oldCache := v.Cache

	debugf("old variable cache: %s", oldCache)

	switch {
	case newValue.Storage == values.Imm:
		v.Cache = newValue
		v.Dirty = true

	case newValue.Storage.IsVarOrStackOrConditionFlags():
		var reg regs.R
		var ok bool

		if oldCache.Storage == values.VarReg {
			reg = oldCache.Reg()
			ok = true
			oldCache.Storage = values.Nowhere // reusing cache register, don't free it
		} else {
			reg, ok = opTryAllocVarReg(f, t)
		}

		if ok {
			zeroExt := opMove(f, reg, newValue, false)
			v.Cache = values.VarRegOperand(t, index, reg, zeroExt)
			v.Dirty = true
		} else {
			// spill to stack
			opStoreVar(f, index, newValue)
			v.Cache = values.NoOperand(t)
			v.Dirty = false
		}

	case newValue.Storage == values.TempReg:
		var reg regs.R
		var zeroExt bool
		var ok bool

		if valueReg := newValue.Reg(); f.Regs.Allocated(t, valueReg) {
			// repurposing the register which already contains the value
			reg = valueReg
			zeroExt = newValue.RegZeroExt()
			ok = true
		} else {
			// can't keep the transient register which contains the value
			if oldCache.Storage == values.VarReg {
				reg = oldCache.Reg()
				ok = true
				oldCache.Storage = values.Nowhere // reusing cache register, don't free it
			} else {
				reg, ok = opTryAllocVarReg(f, t)
			}

			if ok {
				// we got a register for the value
				zeroExt = opMove(f, reg, newValue, false)
			}
		}

		if ok {
			v.Cache = values.VarRegOperand(t, index, reg, zeroExt)
			v.Dirty = true
		} else {
			opStoreVar(f, index, newValue)
			v.Cache = values.NoOperand(t)
			v.Dirty = false
		}

	default:
		panic(newValue)
	}

	if oldCache.Storage == values.VarReg {
		f.Regs.Free(t, oldCache.Reg())
	}
}