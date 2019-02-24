// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package in

type rexWRXB byte

const (
	Rex  = byte(64)
	RexW = rexWRXB(8) // 64-bit operand size
	RexR = rexWRXB(4) // extension of the ModR/M reg field
	RexX = rexWRXB(2) // extension of the SIB index field
	RexB = rexWRXB(1) // extension of the ModR/M r/m field, SIB base field, or Opcode reg field
)

const (
	OneSize = Type(0) // for instructions which don't use RexW
)

const (
	RexMemory = RexB // RegMemoryBase >= 8
)

func typeRexW(t Type) rexWRXB { return rexWRXB(t & 8) } // RexW == 8

func regRexR(r Reg) rexWRXB { return rexWRXB(r>>3) << 2 } // 8..15 => 4
func regRexX(r Reg) rexWRXB { return rexWRXB(r>>3) << 1 } // 8..15 => 2
func regRexB(r Reg) rexWRXB { return rexWRXB(r>>3) << 0 } // 8..15 => 1
