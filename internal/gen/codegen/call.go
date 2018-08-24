// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codegen

import (
	"fmt"

	"github.com/tsavola/wag/abi"
	"github.com/tsavola/wag/internal/gen"
	"github.com/tsavola/wag/internal/links"
	"github.com/tsavola/wag/internal/loader"
	"github.com/tsavola/wag/internal/obj"
	"github.com/tsavola/wag/internal/regalloc"
	"github.com/tsavola/wag/internal/regs"
	"github.com/tsavola/wag/internal/values"
)

func genCall(f *gen.Func, load loader.L, op Opcode, info opInfo) (deadend bool) {
	funcIndex := load.Varuint32()
	if funcIndex >= uint32(len(f.FuncSigs)) {
		panic(fmt.Errorf("%s: function index out of bounds: %d", op, funcIndex))
	}

	sigIndex := f.FuncSigs[funcIndex]
	sig := f.Sigs[sigIndex]

	numStackParams := setupCallOperands(f, op, sig, values.Operand{})

	opCall(f, &f.FuncLinks[funcIndex].L)
	opBackoffStackPtr(f, numStackParams*obj.Word)
	return
}

func genCallIndirect(f *gen.Func, load loader.L, op Opcode, info opInfo) (deadend bool) {
	sigIndex := load.Varuint32()
	if sigIndex >= uint32(len(f.Sigs)) {
		panic(fmt.Errorf("%s: signature index out of bounds: %d", op, sigIndex))
	}

	sig := f.Sigs[sigIndex]

	load.Byte() // reserved

	funcIndex := opMaterializeOperand(f, popOperand(f))
	if funcIndex.Type != abi.I32 {
		panic(fmt.Errorf("%s: function index operand has wrong type: %s", op, funcIndex.Type))
	}

	numStackParams := setupCallOperands(f, op, sig, funcIndex)

	// if funcIndex is a reg, it was already relocated to result reg.
	// otherwise it wasn't touched.
	if !funcIndex.Storage.IsReg() {
		opMove(f, regs.Result, funcIndex, false)
	}

	retAddr := isa.OpCallIndirect(f, int32(len(f.TableFuncs)), int32(sigIndex))
	f.MapCallAddr(retAddr)
	opBackoffStackPtr(f, numStackParams*obj.Word)
	return
}

func setupCallOperands(f *gen.Func, op Opcode, sig abi.Sig, indirect values.Operand) (numStackParams int32) {
	opStackCheck(f)

	args := popOperands(f, len(sig.Args))

	opInitVars(f)
	opSaveTemporaryOperands(f)
	opStoreRegVars(f)

	f.Regs.FreeAll()

	var regArgs regalloc.Map

	for i, value := range args {
		if value.Type != sig.Args[i] {
			panic(fmt.Errorf("%s argument #%d has wrong type: %s", op, i, value.Type))
		}

		var reg regs.R
		var ok bool

		switch value.Storage {
		case values.TempReg:
			reg = value.Reg()
			ok = true

		case values.VarReference:
			if x := f.Vars[value.VarIndex()].Cache; x.Storage == values.VarReg {
				reg = x.Reg()
				ok = true
				args[i] = x // help the next args loop
			}
		}

		if ok {
			f.Regs.SetAllocated(value.Type, reg)
			regArgs.Set(value.Type.Category(), reg, i)
		}
	}

	// relocate indirect index to result reg if it already occupies some reg
	if indirect.Storage.IsReg() && indirect.Reg() != regs.Result {
		if i := regArgs.Get(abi.Int, regs.Result); i >= 0 {
			debugf("indirect call index: %s <-> %s", regs.Result, indirect)
			isa.OpSwap(f.M, abi.Int, regs.Result, indirect.Reg())

			args[i] = values.TempRegOperand(args[i].Type, indirect.Reg(), args[i].RegZeroExt())
			regArgs.Clear(abi.Int, regs.Result)
			regArgs.Set(abi.Int, indirect.Reg(), i)
		} else {
			debugf("indirect call index: %s <- %s", regs.Result, indirect)
			isa.OpMoveReg(f.M, abi.I32, regs.Result, indirect.Reg())
		}
	}

	var paramRegs regalloc.Iterator
	numStackParams = paramRegs.Init(isa.ParamRegs(), sig.Args)

	var numMissingStackArgs int32

	for _, x := range args[:numStackParams] {
		if x.Storage != values.Stack {
			numMissingStackArgs++
		}
	}

	if numMissingStackArgs > 0 {
		opAdvanceStackPtr(f, numMissingStackArgs*obj.Word)

		sourceIndex := numMissingStackArgs
		targetIndex := int32(0)

		// move the register args forward which are currently on stack
		for i := int32(len(args)) - 1; i >= numStackParams; i-- {
			if args[i].Storage == values.Stack {
				debugf("call param #%d: stack (temporary) <- %s", i, args[i])
				isa.OpCopyStack(f.M, targetIndex*obj.Word, sourceIndex*obj.Word)
				sourceIndex++
				targetIndex++
			}
		}

		// move the stack args forward which are already on stack, while
		// inserting the missing stack args
		for i := numStackParams - 1; i >= 0; i-- {
			x := args[i]

			switch x.Storage {
			case values.Stack:
				debugf("call param #%d: stack <- %s", i, x)
				isa.OpCopyStack(f.M, targetIndex*obj.Word, sourceIndex*obj.Word)
				sourceIndex++

			default:
				x = effectiveOperand(f, x)
				debugf("call param #%d: stack <- %s", i, x)
				isa.OpStoreStack(f, targetIndex*obj.Word, x)
			}

			targetIndex++
		}
	}

	// uniquify register operands
	for i, value := range args {
		if value.Storage == values.VarReg {
			cat := value.Type.Category()

			if regArgs.Get(cat, value.Reg()) != i {
				reg, ok := f.Regs.Alloc(value.Type)
				if !ok {
					panic("not enough registers for all register args")
				}

				debugf("call param #%d: %s %s <- %s", i, cat, reg, value.Reg())
				isa.OpMoveReg(f.M, value.Type, reg, value.Reg())

				args[i] = values.RegOperand(false, value.Type, reg)
				regArgs.Set(cat, reg, i)
			}
		}
	}

	f.Regs.FreeAll()

	var preserveFlags bool

	for i := numStackParams; i < int32(len(args)); i++ {
		value := args[i]
		cat := value.Type.Category()
		posReg := paramRegs.IterForward(cat)

		switch {
		case value.Storage.IsReg(): // Vars backed by RegVars were replaced by earlier loop
			if value.Reg() == posReg {
				debugf("call param #%d: %s %s already in place", i, cat, posReg)
			} else {
				if otherArgIndex := regArgs.Get(cat, posReg); otherArgIndex >= 0 {
					debugf("call param #%d: %s %s <-> %s", i, cat, posReg, value.Reg())
					isa.OpSwap(f.M, cat, posReg, value.Reg())

					args[otherArgIndex] = value
					regArgs.Set(cat, value.Reg(), otherArgIndex)
				} else {
					debugf("call param #%d: %s %s <- %s", i, cat, posReg, value.Reg())
					isa.OpMoveReg(f.M, value.Type, posReg, value.Reg())
				}
			}

		case value.Storage == values.ConditionFlags:
			preserveFlags = true
		}
	}

	paramRegs.InitRegs(isa.ParamRegs())

	for i := int32(len(args)) - 1; i >= numStackParams; i-- {
		value := args[i]
		cat := value.Type.Category()
		posReg := paramRegs.IterBackward(cat)

		if !value.Storage.IsReg() {
			debugf("call param #%d: %s %s <- %s", i, cat, posReg, value)
			opMove(f, posReg, value, preserveFlags)
		}
	}

	for i := range f.Vars {
		if v := &f.Vars[i]; v.Cache.Storage == values.VarReg {
			debugf("forget register variable #%d", i)
			// reg was already stored and freed
			v.ResetCache()
		}
	}

	// account for the return address
	if n := f.StackOffset + obj.Word; n > f.MaxStackOffset {
		f.MaxStackOffset = n
	}

	if sig.Result != abi.Void {
		pushResultRegOperand(f, sig.Result)
	}

	return
}

func opCall(f *gen.Func, l *links.L) {
	retAddr := isa.OpCall(f.M, l.Addr)
	f.MapCallAddr(retAddr)
	if l.Addr == 0 {
		l.AddSite(retAddr)
	}
}